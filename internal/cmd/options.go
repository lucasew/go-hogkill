package cmd

import (
	"fmt"
	"os/user"
	"strings"
	"time"

	"github.com/lucasew/go-hogkill/internal/proc"
)

// Options are shared CLI settings.
type Options struct {
	Sort      proc.SortKey
	Interval  time.Duration
	Top       int
	MinCPU    float64
	MinMemMB  float64
	User      string
	Me        bool
	Filter    string
	SafeOnly  bool
	NoColor   bool
	DryRun    bool
	Flat      bool
	JSON      bool
	Yes       bool
	Force     bool
	Escalate  time.Duration
}

func defaultOptions() Options {
	return Options{
		Sort:     proc.SortCPU,
		Interval: 1500 * time.Millisecond,
		Top:      20,
		Escalate: 4 * time.Second,
	}
}

func (o *Options) normalize(args []string) error {
	if o.Me {
		u, err := user.Current()
		if err != nil {
			return fmt.Errorf("resolve --me: %w", err)
		}
		o.User = u.Username
	}
	if o.User != "" && !proc.SupportsUsers {
		return fmt.Errorf("--user and --me need process ownership, which this platform does not report cheaply")
	}
	if len(args) > 0 && o.Filter == "" {
		o.Filter = strings.Join(args, " ")
	} else if len(args) > 0 {
		o.Filter = strings.TrimSpace(o.Filter + " " + strings.Join(args, " "))
	}
	switch o.Sort {
	case proc.SortCPU, proc.SortMem, proc.SortCount, proc.SortName:
	default:
		return fmt.Errorf("unknown sort %q — use cpu, mem, count, name", o.Sort)
	}
	if o.Interval < 250*time.Millisecond {
		o.Interval = 250 * time.Millisecond
	}
	return nil
}

func (o Options) minMemBytes() uint64 {
	if o.MinMemMB <= 0 {
		return 0
	}
	return uint64(o.MinMemMB * 1024 * 1024)
}

func (o Options) groupOpts() proc.GroupOptions {
	return proc.GroupOptions{
		MinCPU:   o.MinCPU,
		MinMem:   o.minMemBytes(),
		User:     o.User,
		Filter:   o.Filter,
		SafeOnly: o.SafeOnly,
	}
}
