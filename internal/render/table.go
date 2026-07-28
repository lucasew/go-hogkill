package render

import (
	"fmt"
	"io"

	"github.com/lucasew/go-hogkill/internal/proc"
)

// TableOptions for one-shot listing.
type TableOptions struct {
	Top      int
	Flat     bool
	NoColor  bool
	WithUser bool
}

// WriteTable prints groups as a text table.
func WriteTable(w io.Writer, groups []proc.Group, opt TableOptions) {
	withUser := opt.WithUser && proc.SupportsUsers
	nameWidth := 40
	if !withUser {
		nameWidth = 48
	}

	fmt.Fprintf(w, "%s%s%s%s  %s%s\n",
		Fit("NAME", nameWidth),
		PadStart("CPU", 7),
		PadStart("MEMORY", 11),
		PadStart("PROCS", 6),
		Fit("RISK", 9),
		userHeader(withUser),
	)

	n := opt.Top
	if n <= 0 || n > len(groups) {
		n = len(groups)
	}
	shown := groups[:n]
	risky := false
	for _, g := range shown {
		if g.Risk != proc.RiskNone {
			risky = true
		}
		fmt.Fprintf(w, "%s%s%s%s  %s%s\n",
			Fit(g.Name, nameWidth),
			PadStart(Percent(g.CPU), 7),
			PadStart(Bytes(g.RSS), 11),
			PadStart(fmt.Sprintf("%d", len(g.Procs)), 6),
			Fit(proc.RiskTag[g.Risk], 9),
			userCell(withUser, g.User),
		)
		if !opt.Flat {
			continue
		}
		for _, p := range g.Procs {
			label := fmt.Sprintf("%d %s", p.PID, p.Name)
			fmt.Fprintf(w, "  %s%s%s%s  %s\n",
				Fit(label, nameWidth-2),
				PadStart(Percent(p.CPU), 7),
				PadStart(Bytes(p.RSS), 11),
				"      ",
				Fit(proc.RiskTag[p.Risk], 9),
			)
		}
	}
	if risky {
		fmt.Fprintln(w, "\nrisk: what breaks if you kill it — run with --json to read the reason")
	}
}

func userHeader(with bool) string {
	if with {
		return "USER"
	}
	return ""
}

func userCell(with bool, user string) string {
	if with {
		return user
	}
	return ""
}
