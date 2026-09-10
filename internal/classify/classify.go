// Package classify assigns severity to differences and removes the ones the
// user has chosen to ignore.
package classify

import (
	"strings"

	"github.com/4ugane/kubectl-env-diff/internal/config"
	"github.com/4ugane/kubectl-env-diff/internal/model"
)

// Apply returns a new slice with ignored differences removed and Severity set
// on the rest. The input is not modified.
//
// Order is preserved, because compare.DiffAll has already imposed the total
// ordering the report depends on.
func Apply(diffs []model.Difference, rules []config.IgnoreRule) []model.Difference {
	out := make([]model.Difference, 0, len(diffs))
	for _, d := range diffs {
		if ignored(d, rules) {
			continue
		}
		d.Severity = severity(d)
		out = append(out, d)
	}
	return out
}

// severity decides whether a difference is routine.
//
// The Expected set is intentionally tiny: a differing image tag and a differing
// replica count are normal between environments. Nothing else is excused.
// Detecting namespace strings, account IDs, or hostnames inside values was
// considered and rejected, because a false "expected" hides a real fault.
func severity(d model.Difference) model.Severity {
	// Only a CHANGED value is routine. A missing image tag or replica field
	// means the setting is absent, which is a genuine problem.
	if d.Type != model.ValueChanged {
		return model.SeverityDrift
	}
	if d.Path == "replicas" {
		return model.SeverityExpected
	}
	if strings.HasSuffix(d.Path, ".image.tag") {
		return model.SeverityExpected
	}
	return model.SeverityDrift
}

// ignored reports whether any rule suppresses this difference. Within one rule
// every populated field must match; an unset field matches anything.
func ignored(d model.Difference, rules []config.IgnoreRule) bool {
	for _, r := range rules {
		if r.Kind != "" && string(d.Kind) != r.Kind {
			continue
		}
		if r.Name != "" && d.Name != r.Name {
			continue
		}
		if r.Path != "" && !config.MatchPath(r.Path, d.Path) {
			continue
		}
		return true
	}
	return false
}
