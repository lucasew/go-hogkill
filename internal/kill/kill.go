package kill

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"
)

// ImmediateOnly is true on Windows (no signals).
var ImmediateOnly = runtime.GOOS == "windows"

// Options control signalling.
type Options struct {
	Force         bool
	EscalateAfter time.Duration
	DryRun        bool
}

// Target is one process to signal.
type Target struct {
	PID  int32
	Name string
	Own  bool
}

// Status of a kill attempt.
type Status string

const (
	StatusTerminated Status = "terminated"
	StatusKilled     Status = "killed"
	StatusSurvived   Status = "survived"
	StatusDenied     Status = "denied"
	StatusGone       Status = "gone"
	StatusDryRun     Status = "dry-run"
)

// Outcome is the result for one PID.
type Outcome struct {
	PID    int32
	Name   string
	Status Status
	Error  string
}

// KillVerb label for UI/prompts.
func KillVerb(force bool) string {
	if ImmediateOnly {
		return "TERMINATE"
	}
	if force {
		return "SIGKILL"
	}
	return "SIGTERM"
}

// IsAlive reports whether pid still exists (or we lack permission to probe).
func IsAlive(pid int32) bool {
	if pid <= 0 {
		return true
	}
	p, err := os.FindProcess(int(pid))
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// EPERM means it exists.
	if err == syscall.EPERM {
		return true
	}
	return false
}

func send(pid int32, sig syscall.Signal) (sent bool, gone bool, denied bool) {
	if pid <= 0 {
		return false, false, true
	}
	p, err := os.FindProcess(int(pid))
	if err != nil {
		return false, true, false
	}
	if ImmediateOnly || sig == syscall.SIGKILL {
		err = p.Kill()
	} else {
		err = p.Signal(sig)
	}
	if err == nil {
		return true, false, false
	}
	if err == os.ErrProcessDone || err == syscall.ESRCH {
		return false, true, false
	}
	// Windows may return different errors for gone processes.
	if pe, ok := err.(*os.SyscallError); ok && pe.Err == syscall.ESRCH {
		return false, true, false
	}
	return false, false, true
}

// Targets signals processes: SIGTERM then optional SIGKILL, own last.
func Targets(targets []Target, opt Options) []Outcome {
	ordered := append([]Target(nil), targets...)
	// own last
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			if ordered[i].Own && !ordered[j].Own {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
	}

	outcomes := make(map[int32]Outcome, len(ordered))
	var pending []Target

	for _, t := range ordered {
		if opt.DryRun {
			outcomes[t.PID] = Outcome{PID: t.PID, Name: t.Name, Status: StatusDryRun}
			continue
		}
		immediate := opt.Force || ImmediateOnly
		sig := syscall.SIGTERM
		if immediate {
			sig = syscall.SIGKILL
		}
		sent, gone, denied := send(t.PID, sig)
		switch {
		case gone:
			outcomes[t.PID] = Outcome{PID: t.PID, Name: t.Name, Status: StatusGone}
		case denied:
			msg := "permission denied — rerun with sudo"
			if t.PID <= 0 {
				msg = "pid 0 is the kernel, not a killable process"
			}
			outcomes[t.PID] = Outcome{PID: t.PID, Name: t.Name, Status: StatusDenied, Error: msg}
		case sent:
			st := StatusTerminated
			if immediate {
				st = StatusKilled
			}
			outcomes[t.PID] = Outcome{PID: t.PID, Name: t.Name, Status: st}
			if !immediate {
				pending = append(pending, t)
			}
		}
	}

	if len(pending) > 0 && opt.EscalateAfter > 0 {
		time.Sleep(opt.EscalateAfter)
		for _, t := range pending {
			if !IsAlive(t.PID) {
				continue
			}
			sent, _, denied := send(t.PID, syscall.SIGKILL)
			switch {
			case sent:
				outcomes[t.PID] = Outcome{PID: t.PID, Name: t.Name, Status: StatusKilled}
			case denied:
				outcomes[t.PID] = Outcome{PID: t.PID, Name: t.Name, Status: StatusSurvived, Error: "permission denied — rerun with sudo"}
			default:
				outcomes[t.PID] = Outcome{PID: t.PID, Name: t.Name, Status: StatusSurvived}
			}
		}
	}

	out := make([]Outcome, 0, len(outcomes))
	for _, t := range ordered {
		if o, ok := outcomes[t.PID]; ok {
			out = append(out, o)
		}
	}
	return out
}

// Summarize human message for a kill batch.
func Summarize(outcomes []Outcome, subject string) string {
	if len(outcomes) == 0 {
		return "nothing to kill"
	}
	for _, o := range outcomes {
		if o.Status == StatusDryRun {
			return fmt.Sprintf("dry run — %s (%d) would be killed", subject, len(outcomes))
		}
	}
	var down, denied, survived int
	for _, o := range outcomes {
		switch o.Status {
		case StatusTerminated, StatusKilled, StatusGone:
			down++
		case StatusDenied:
			denied++
		case StatusSurvived:
			survived++
		}
	}
	var parts []string
	if down > 0 {
		parts = append(parts, fmt.Sprintf("%s (%d) killed!", subject, down))
	} else if denied > 0 || survived > 0 {
		parts = append(parts, subject+" survived")
	}
	if denied > 0 {
		parts = append(parts, fmt.Sprintf("%d denied (needs sudo)", denied))
	}
	if survived > 0 && down > 0 {
		parts = append(parts, fmt.Sprintf("%d still alive", survived))
	}
	if len(parts) == 0 {
		return "nothing to kill"
	}
	msg := parts[0]
	for i := 1; i < len(parts); i++ {
		msg += " · " + parts[i]
	}
	return msg
}
