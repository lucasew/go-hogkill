package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/lucasew/go-hogkill/internal/cmd"
)

// Filled by goreleaser ldflags.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

func main() {
	cmd.Version = version
	cmd.Commit = commit
	cmd.Date = date
	if err := cmd.Execute(); err != nil {
		var ex interface{ ExitCode() int }
		if errors.As(err, &ex) {
			// Message already printed by the command (e.g. no match).
			os.Exit(ex.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "hk:", err)
		os.Exit(1)
	}
}
