// Package cli defines the command surface and orchestrates a run.
package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

// Options is the parsed flag set.
type Options struct {
	From         string
	To           string
	Name         string
	FromName     string
	ToName       string
	Kinds        []string
	Output       string
	ConfigPath   string
	ConfigSet    bool
	ShowExpected bool
	Full         bool
	NoColor      bool
}

var validOutputs = map[string]bool{"text": true, "json": true, "markdown": true, "html": true}

var kindAliases = map[string]model.Kind{
	"deployment":   model.KindDeployment,
	"deployments":  model.KindDeployment,
	"deploy":       model.KindDeployment,
	"statefulset":  model.KindStatefulSet,
	"statefulsets": model.KindStatefulSet,
	"sts":          model.KindStatefulSet,
	"configmap":    model.KindConfigMap,
	"configmaps":   model.KindConfigMap,
	"cm":           model.KindConfigMap,
}

// Validate rejects flag combinations that cannot be honoured. Everything here
// fails before a single API call, so a typo never costs a round trip.
func (o *Options) Validate() error {
	if o.From == "" {
		return fmt.Errorf("--from is required (context[/namespace])")
	}
	if o.To == "" {
		return fmt.Errorf("--to is required (context[/namespace])")
	}
	if o.From == o.To {
		return fmt.Errorf("--from and --to are both %q; that compares a namespace to itself", o.From)
	}

	explicit := o.FromName != "" || o.ToName != ""
	if o.Name != "" && explicit {
		return fmt.Errorf("--name cannot be combined with --from-name/--to-name; " +
			"use --name when both sides share a name, or --from-name and --to-name when they differ")
	}
	if explicit && (o.FromName == "" || o.ToName == "") {
		return fmt.Errorf("--from-name and --to-name must be given together")
	}

	if o.Output != "" && !validOutputs[o.Output] {
		return fmt.Errorf("unknown --output %q (want text, json, markdown, or html)", o.Output)
	}

	for _, k := range o.Kinds {
		if _, ok := kindAliases[strings.ToLower(k)]; !ok {
			return fmt.Errorf("unknown --kind %q (want deployment, statefulset, or configmap)", k)
		}
	}
	return nil
}

// ResolvedKinds maps the kind flags onto model kinds, defaulting to all three.
func (o *Options) ResolvedKinds() ([]model.Kind, error) {
	if len(o.Kinds) == 0 {
		return []model.Kind{model.KindDeployment, model.KindStatefulSet, model.KindConfigMap}, nil
	}
	seen := map[model.Kind]bool{}
	for _, k := range o.Kinds {
		resolved, ok := kindAliases[strings.ToLower(k)]
		if !ok {
			return nil, fmt.Errorf("unknown --kind %q", k)
		}
		seen[resolved] = true
	}
	out := make([]model.Kind, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// ExitCode maps a run's outcome onto a process exit code.
//
// Incompleteness outranks drift. A run that could not read every resource did
// not answer the question, so reporting 2 ("drift found") would overstate what
// is actually known.
func ExitCode(hasDrift, incomplete bool) int {
	switch {
	case incomplete:
		return 1
	case hasDrift:
		return 2
	default:
		return 0
	}
}
