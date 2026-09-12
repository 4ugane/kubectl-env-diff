package cli

import (
	"github.com/spf13/cobra"
)

// NewCommand builds the root command.
func NewCommand(version string) *cobra.Command {
	o := &Options{}

	cmd := &cobra.Command{
		Use:   "kubectl-env_diff",
		Short: "Compare workload configuration between two live Kubernetes clusters",
		Long: `Compare Deployments, StatefulSets, ConfigMaps, and non-sensitive
environment variables between two kubeconfig contexts.

Secret values are never read: the tool requires no permission on secrets and
reports only the names and keys that pod specs reference.

Exit codes: 0 no drift, 2 drift found, 1 tool error or incomplete comparison.`,
		Example: `  # whole namespace
  kubectl env-diff --from staging/web --to prod-au/web

  # one workload, same name on both sides
  kubectl env-diff --from staging/web --to prod/web --name checkout

  # names differ between environments
  kubectl env-diff --from staging/web --to prod/web \
      --from-name test-staging --to-name test-prod

  # shareable report
  kubectl env-diff --from staging/web --to prod/web --output html > drift.html`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			o.ConfigSet = cmd.Flags().Changed("config")
			code := Run(cmd.Context(), o, cmd.OutOrStdout(), cmd.ErrOrStderr())
			if code != 0 {
				return &exitError{code: code}
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&o.From, "from", "", "source context[/namespace]")
	f.StringVar(&o.To, "to", "", "target context[/namespace]")
	f.StringVar(&o.Name, "name", "", "compare only this resource name on both sides")
	f.StringVar(&o.FromName, "from-name", "", "resource name on the source side (use with --to-name)")
	f.StringVar(&o.ToName, "to-name", "", "resource name on the target side (use with --from-name)")
	f.StringSliceVar(&o.Kinds, "kind", nil, "kinds to compare (default deployment,statefulset,configmap)")
	f.StringVarP(&o.Output, "output", "o", "text", "output format: text, json, markdown, html")
	f.StringVar(&o.ConfigPath, "config", "", "path to .envdiff.yaml")
	f.BoolVar(&o.ShowExpected, "show-expected", false, "show expected differences instead of collapsing them")
	f.BoolVar(&o.Full, "full", false, "do not truncate long values in text output")
	f.BoolVar(&o.NoColor, "no-color", false, "disable coloured output")

	return cmd
}

// exitError carries a specific exit code out of RunE without printing twice.
type exitError struct{ code int }

func (e *exitError) Error() string { return "" }

// ExitCodeOf extracts the code from an error returned by the root command.
func ExitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exitError); ok {
		return ee.code
	}
	return 1
}
