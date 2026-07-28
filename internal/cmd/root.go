package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/lucasew/go-hogkill/internal/proc"
	"github.com/lucasew/go-hogkill/internal/tui"
	"github.com/spf13/cobra"
)

// Version is set from main via ldflags-friendly vars.
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// Execute runs the CLI.
func Execute() error {
	return New().Execute()
}

// New builds the root command tree.
func New() *cobra.Command {
	opts := defaultOptions()

	root := &cobra.Command{
		Use:   "hk [filter]",
		Short: "find and kill processes eating your machine",
		Long: `hk is hogkill for the terminal: one row per app, live CPU, kill with one key.

  hk                     interactive view, apps sorted by CPU
  hk -m                  same, sorted by memory
  hk top --top 15        print the top 15 and exit
  hk kill Slack -y       kill matching apps without a prompt`,
		Args:                  cobra.ArbitraryArgs,
		DisableFlagsInUseLine: false,
		SilenceUsage:          true,
		SilenceErrors:         true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.normalize(args); err != nil {
				return err
			}
			// Non-TTY: fall back to one-shot list.
			if !isInteractive() {
				return runTop(cmd.Context(), opts)
			}
			return tui.Run(tui.Options{
				Interval:      opts.Interval,
				Sort:          opts.Sort,
				MinCPU:        opts.MinCPU,
				MinMem:        opts.minMemBytes(),
				User:          opts.User,
				Filter:        opts.Filter,
				SafeOnly:      opts.SafeOnly,
				DryRun:        opts.DryRun,
				EscalateAfter: opts.Escalate,
			})
		},
	}

	pf := root.PersistentFlags()
	pf.StringVarP((*string)(&opts.Sort), "sort", "s", string(proc.SortCPU), "cpu | mem | count | name")
	pf.BoolP("mem", "m", false, "shortcut for --sort mem")
	pf.DurationVarP(&opts.Interval, "interval", "i", opts.Interval, "refresh rate")
	pf.Float64Var(&opts.MinCPU, "min-cpu", 0, "hide apps below this CPU%")
	pf.Float64Var(&opts.MinMemMB, "min-mem", 0, "hide apps below this memory (MB)")
	pf.StringVarP(&opts.User, "user", "u", "", "only this user's processes")
	pf.BoolVar(&opts.Me, "me", false, "only your own processes")
	pf.StringVarP(&opts.Filter, "filter", "f", "", "only apps matching text")
	pf.BoolVar(&opts.SafeOnly, "safe-only", false, "hide processes the system depends on")
	pf.BoolVar(&opts.NoColor, "no-color", false, "plain output")
	pf.BoolVar(&opts.DryRun, "dry-run", false, "show what would die, kill nothing")

	// --mem as persistent pre-parse via flag lookup
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if m, _ := cmd.Flags().GetBool("mem"); m {
			opts.Sort = proc.SortMem
		}
		return nil
	}

	root.AddCommand(newTopCmd(&opts))
	root.AddCommand(newKillCmd(&opts))
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", Version)
		},
	})

	root.Version = Version
	root.SetVersionTemplate("{{.Version}}\n")

	return root
}

func isInteractive() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	fi, err = os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func collect(ctx context.Context, opts Options) ([]proc.Group, error) {
	var s proc.Sampler
	procs, err := s.SampleTwice(ctx, 600*time.Millisecond)
	if err != nil {
		return nil, err
	}
	groups := proc.GroupProcesses(procs, opts.groupOpts())
	return proc.SortGroups(groups, opts.Sort), nil
}
