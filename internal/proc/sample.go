package proc

import (
	"context"
	"math"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/process"
)

// SupportsUsers is false on platforms where ownership is expensive/unavailable.
var SupportsUsers = runtime.GOOS != "windows"

// Cores is logical CPU count for scaling header bars.
func Cores() int {
	n, err := cpu.Counts(true)
	if err != nil || n < 1 {
		return runtime.NumCPU()
	}
	return n
}

const smoothing = 0.6

// Sampler diffs CPU times between samples and smooths the result.
type Sampler struct {
	prevAt       time.Time
	prevCPU      map[int32]float64
	prevSmoothed map[int32]float64
}

// Sample collects processes with live CPU estimates.
func (s *Sampler) Sample(ctx context.Context) ([]Proc, error) {
	list, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	at := time.Now()
	since := 0.0
	if !s.prevAt.IsZero() {
		since = at.Sub(s.prevAt).Seconds()
	}

	maxCPU := float64(Cores()) * 100
	cpuSeconds := make(map[int32]float64, len(list))
	smoothed := make(map[int32]float64, len(list))
	procs := make([]Proc, 0, len(list))

	for _, p := range list {
		pid := p.Pid
		ppid, _ := p.PpidWithContext(ctx)
		var rss uint64
		if mi, err := p.MemoryInfoWithContext(ctx); err == nil && mi != nil {
			rss = mi.RSS
		}
		var cpuSec float64
		if t, err := p.TimesWithContext(ctx); err == nil && t != nil {
			cpuSec = t.User + t.System + t.Iowait + t.Irq + t.Softirq + t.Steal + t.Nice
		}
		cpuSeconds[pid] = cpuSec

		var fallback float64
		if pct, err := p.CPUPercentWithContext(ctx); err == nil {
			fallback = pct
		}

		cpuVal := fallback
		if before, ok := s.prevCPU[pid]; ok && since > 0.2 && cpuSec >= before {
			cpuVal = ((cpuSec - before) / since) * 100
		}
		cpuVal = math.Min(maxCPU, math.Max(0, cpuVal))
		if prev, ok := s.prevSmoothed[pid]; ok {
			cpuVal = prev*(1-smoothing) + cpuVal*smoothing
		}
		smoothed[pid] = cpuVal

		var elapsed float64
		if ct, err := p.CreateTimeWithContext(ctx); err == nil && ct > 0 {
			elapsed = float64(at.UnixMilli()-ct) / 1000
			if elapsed < 0 {
				elapsed = 0
			}
		}

		user := ""
		if SupportsUsers {
			user, _ = p.UsernameWithContext(ctx)
		}

		cmdline, _ := p.CmdlineWithContext(ctx)
		exe, _ := p.ExeWithContext(ctx)
		if exe == "" {
			exe = ResolveExe(cmdline, "")
		}
		name, _ := p.NameWithContext(ctx)
		if name == "" {
			name = BaseName(exe)
		}

		procs = append(procs, Proc{
			PID:        pid,
			PPID:       ppid,
			RSS:        rss,
			CPU:        math.Round(cpuVal*10) / 10,
			CPUSeconds: cpuSec,
			Elapsed:    elapsed,
			User:       user,
			Command:    cmdline,
			Exe:        exe,
			Name:       DisplayName(cmdline, exe),
			Risk:       RiskNone,
		})
		_ = name
	}

	s.prevAt = at
	s.prevCPU = cpuSeconds
	s.prevSmoothed = smoothed
	markRisk(procs)
	return procs, nil
}

// SampleTwice takes two samples spaced by wait so non-interactive CPU is meaningful.
func (s *Sampler) SampleTwice(ctx context.Context, wait time.Duration) ([]Proc, error) {
	if _, err := s.Sample(ctx); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(wait):
	}
	return s.Sample(ctx)
}

func markRisk(procs []Proc) {
	byPID := make(map[int32]Proc, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
	}

	lineage := map[int32]struct{}{}
	cursor := int32(os.Getpid())
	for cursor > 0 {
		if _, seen := lineage[cursor]; seen {
			break
		}
		lineage[cursor] = struct{}{}
		parent, ok := byPID[cursor]
		if !ok {
			break
		}
		cursor = parent.PPID
	}

	for i := range procs {
		level, reason := AssessRisk(procs[i], lineage)
		procs[i].Risk = level
		procs[i].RiskReason = reason
		// Prefer DisplayName; fall back already set.
		if procs[i].Name == "" {
			procs[i].Name = BaseName(procs[i].Exe)
		}
	}
}
