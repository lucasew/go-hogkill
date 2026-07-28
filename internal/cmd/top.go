package cmd

import (
	"context"
	"os"

	"github.com/lucasew/go-hogkill/internal/proc"
	"github.com/lucasew/go-hogkill/internal/render"
	"github.com/spf13/cobra"
)

func newTopCmd(opts *Options) *cobra.Command {
	c := &cobra.Command{
		Use:     "top",
		Aliases: []string{"ls", "list"},
		Short:   "print ranked apps once and exit",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.normalize(args); err != nil {
				return err
			}
			return runTop(cmd.Context(), *opts)
		},
	}
	c.Flags().IntVarP(&opts.Top, "top", "n", 20, "rows to print")
	c.Flags().BoolVar(&opts.Flat, "flat", false, "list individual processes under each app")
	c.Flags().BoolVar(&opts.JSON, "json", false, "machine readable output")
	return c
}

func runTop(ctx context.Context, opts Options) error {
	groups, err := collect(ctx, opts)
	if err != nil {
		return err
	}
	if opts.JSON {
		return render.WriteJSON(os.Stdout, groups, opts.Top)
	}
	render.WriteTable(os.Stdout, groups, render.TableOptions{
		Top:      opts.Top,
		Flat:     opts.Flat,
		NoColor:  opts.NoColor,
		WithUser: proc.SupportsUsers,
	})
	return nil
}
