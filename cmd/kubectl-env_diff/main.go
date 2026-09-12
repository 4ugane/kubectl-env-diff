// Command kubectl-env_diff compares workload configuration between two live
// Kubernetes clusters.
//
// The underscore in the binary name is what lets kubectl expose it as
// "kubectl env-diff"; kubectl maps underscores in plugin names to dashes.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/4ugane/kubectl-env-diff/internal/cli"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	cmd := cli.NewCommand(version)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		if msg := err.Error(); msg != "" {
			fmt.Fprintf(os.Stderr, "error: %s\n", msg)
		}
		os.Exit(cli.ExitCodeOf(err))
	}
}
