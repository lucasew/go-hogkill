package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/lucasew/go-hogkill/internal/kill"
	"github.com/lucasew/go-hogkill/internal/proc"
	"github.com/lucasew/go-hogkill/internal/render"
	"github.com/spf13/cobra"
)

func newKillCmd(opts *Options) *cobra.Command {
	c := &cobra.Command{
		Use:     "kill [pattern]",
		Aliases: []string{"rm"},
		Short:   "non-interactive kill by name match",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.normalize(nil); err != nil {
				return err
			}
			pattern := strings.Join(args, " ")
			return runKill(cmd, *opts, pattern)
		},
	}
	c.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "skip the confirmation prompt")
	c.Flags().BoolVarP(&opts.Force, "force", "9", false, "SIGKILL immediately (no-op escalation on Windows)")
	return c
}

func runKill(cmd *cobra.Command, opts Options, pattern string) error {
	groups, err := collect(cmd.Context(), opts)
	if err != nil {
		return err
	}
	needle := strings.ToLower(strings.TrimSpace(pattern))
	if needle == "" {
		return fmt.Errorf("kill pattern required")
	}
	var batches []proc.Group
	var procs []proc.Proc
	for _, g := range groups {
		// Never match own session: the typed pattern sits in hk/shell argv too.
		kept := make([]proc.Proc, 0, len(g.Procs))
		for _, p := range g.Procs {
			if p.Risk == proc.RiskOwn {
				continue
			}
			nameHit := strings.Contains(strings.ToLower(g.Name), needle) ||
				strings.Contains(strings.ToLower(p.Name), needle)
			cmdHit := strings.Contains(strings.ToLower(p.Command), needle)
			if nameHit || cmdHit {
				kept = append(kept, p)
			}
		}
		if len(kept) == 0 {
			continue
		}
		g.Procs = kept
		g.Risk = proc.HighestRisk(kept)
		if g.Risk != proc.RiskNone {
			for _, p := range kept {
				if p.Risk == g.Risk {
					g.RiskReason = p.RiskReason
					break
				}
			}
		} else {
			g.RiskReason = ""
		}
		batches = append(batches, g)
		procs = append(procs, kept...)
	}
	if len(procs) == 0 {
		fmt.Fprintf(os.Stderr, "no match for %q\n", pattern)
		return errExit{1}
	}

	var reclaimed uint64
	for _, p := range procs {
		reclaimed += p.RSS
	}
	fmt.Fprintf(cmd.OutOrStdout(), "about to kill %d process%s · %s\n",
		len(procs), processPlural(len(procs)), render.Bytes(reclaimed))
	for _, b := range batches {
		var size uint64
		for _, p := range b.Procs {
			size += p.RSS
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %-9s %s — %d proc%s, %s\n",
			proc.RiskTag[b.Risk], b.Name, len(b.Procs), plural(len(b.Procs)), render.Bytes(size))
	}
	printWarnings(cmd, procs)

	if !opts.Yes && !opts.DryRun {
		if !isInteractive() {
			return fmt.Errorf("refusing to kill without --yes when stdin is not a terminal")
		}
		fmt.Fprint(cmd.OutOrStdout(), "proceed? [y/N] ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		line = strings.TrimSpace(line)
		if !strings.EqualFold(line, "y") && !strings.EqualFold(line, "yes") {
			fmt.Fprintln(cmd.OutOrStdout(), "cancelled")
			return errExit{130}
		}
	}

	targets := make([]kill.Target, 0, len(procs))
	for _, p := range procs {
		targets = append(targets, kill.Target{
			PID:  p.PID,
			Name: fmt.Sprintf("%s (%d)", p.Name, p.PID),
		})
	}
	esc := opts.Escalate
	if opts.Force {
		esc = 0
	}
	outcomes := kill.Targets(targets, kill.Options{
		Force:         opts.Force,
		EscalateAfter: esc,
		DryRun:        opts.DryRun,
	})
	subject := fmt.Sprintf("%d apps", len(batches))
	if len(batches) == 1 {
		subject = batches[0].Name
	}
	fmt.Fprintln(cmd.OutOrStdout(), kill.Summarize(outcomes, subject))
	for _, o := range outcomes {
		if o.Status == kill.StatusDenied || o.Status == kill.StatusSurvived {
			return errExit{1}
		}
	}
	return nil
}

func printWarnings(cmd *cobra.Command, procs []proc.Proc) {
	warnings := proc.CollectWarnings(procs)
	if len(warnings) == 0 {
		return
	}
	risk := proc.HighestRisk(procs)
	headline := "this batch includes processes the system uses:"
	if risk == proc.RiskCritical {
		headline = "this batch includes critical system processes:"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", headline)
	for _, w := range warnings {
		fmt.Fprintf(cmd.OutOrStdout(), "  %-9s %s — %s\n", proc.RiskTag[w.Level], w.Name, w.Reason)
	}
	fmt.Fprintln(cmd.OutOrStdout())
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func processPlural(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}

type errExit struct{ code int }

func (e errExit) Error() string { return fmt.Sprintf("exit %d", e.code) }
func (e errExit) ExitCode() int { return e.code }
