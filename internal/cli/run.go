package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"

	"github.com/4ugane/kubectl-env-diff/internal/classify"
	"github.com/4ugane/kubectl-env-diff/internal/compare"
	"github.com/4ugane/kubectl-env-diff/internal/config"
	"github.com/4ugane/kubectl-env-diff/internal/kube"
	"github.com/4ugane/kubectl-env-diff/internal/model"
	"github.com/4ugane/kubectl-env-diff/internal/report"
)

// Run performs one comparison and returns the process exit code.
func Run(ctx context.Context, o *Options, stdout, stderr io.Writer) int {
	if err := o.Validate(); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	fromTarget, err := kube.ParseTarget(o.From)
	if err != nil {
		fmt.Fprintf(stderr, "error: --from: %v\n", err)
		return 1
	}
	toTarget, err := kube.ParseTarget(o.To)
	if err != nil {
		fmt.Fprintf(stderr, "error: --to: %v\n", err)
		return 1
	}

	fromClient, fromTarget, err := kube.ClientFor(fromTarget)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	toClient, toTarget, err := kube.ClientFor(toTarget)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	// Namespaces may have been filled in from kubeconfig, so re-check that the
	// two sides did not converge on the same target.
	if fromTarget == toTarget {
		fmt.Fprintf(stderr,
			"error: both sides resolve to %s/%s; that compares a namespace to itself\n",
			fromTarget.Context, fromTarget.Namespace)
		return 1
	}

	kinds, err := o.ResolvedKinds()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	cfgPath := o.ConfigPath
	if cfgPath == "" {
		cfgPath = config.DefaultPath
	}
	cfg, err := config.Load(cfgPath, o.ConfigSet)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	fromSnap, err := kube.Fetch(ctx, fromClient, fromTarget.Context, fromTarget.Namespace, kinds)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n\n%s\n", err, kube.MinimalRole)
		return 1
	}
	toSnap, err := kube.Fetch(ctx, toClient, toTarget.Context, toTarget.Namespace, kinds)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n\n%s\n", err, kube.MinimalRole)
		return 1
	}

	skipped := append(append([]model.SkipNote{}, fromSnap.Skipped...), toSnap.Skipped...)
	skippedKinds := map[model.Kind]bool{}
	for _, s := range skipped {
		skippedKinds[s.Kind] = true
	}
	fromSnap = fromSnap.WithoutKinds(skippedKinds)
	toSnap = toSnap.WithoutKinds(skippedKinds)

	pairs, err := compare.Pairs(fromSnap, toSnap, compare.Options{
		Name:      o.Name,
		FromName:  o.FromName,
		ToName:    o.ToName,
		Normalize: cfg.Normalize,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	diffs := classify.Apply(compare.DiffAll(pairs), cfg.Ignore)
	rep := report.Build(report.Meta{
		FromContext: fromTarget.Context, FromNamespace: fromTarget.Namespace,
		ToContext: toTarget.Context, ToNamespace: toTarget.Namespace,
	}, pairs, diffs, skipped)

	if err := render(stdout, rep, o); err != nil {
		fmt.Fprintf(stderr, "error: writing output: %v\n", err)
		return 1
	}
	return ExitCode(rep.HasDrift(), rep.Incomplete())
}

// render dispatches to the selected renderer.
func render(w io.Writer, rep report.Report, o *Options) error {
	switch o.Output {
	case "json":
		return report.JSON(w, rep)
	case "markdown":
		return report.Markdown(w, rep, o.ShowExpected)
	case "html":
		return report.HTML(w, rep, o.ShowExpected)
	default:
		return report.Text(w, rep, report.TextOptions{
			Width:        terminalWidth(w),
			Color:        useColor(w, o.NoColor),
			ShowExpected: o.ShowExpected,
			Full:         o.Full,
		})
	}
}

// terminalWidth returns the usable width, or 0 when output is not a terminal so
// the renderer falls back to a fixed width and piped output stays stable.
func terminalWidth(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok {
		return 0
	}
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil || width <= 0 {
		return 0
	}
	return width
}

// useColor enables ANSI only on a real terminal, and never when NO_COLOR is
// set, per the no-color.org convention.
func useColor(w io.Writer, disabled bool) bool {
	if disabled {
		return false
	}
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}
