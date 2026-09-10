// Package compare pairs resources across two snapshots and diffs them field by
// field. It operates entirely on model types — no Kubernetes imports.
package compare

import (
	"fmt"
	"strings"

	"github.com/4ugane/kubectl-env-diff/internal/config"
	"github.com/4ugane/kubectl-env-diff/internal/model"
)

// NormalizeName strips configured environment affixes so that api-staging and
// api-prod pair with each other.
//
// The longest matching affix wins. Without that rule, overlapping entries like
// "-staging" and "-staging-canary" would produce different results depending on
// config file ordering, which is exactly the kind of nondeterminism that makes a
// diff untrustworthy.
func NormalizeName(name string, rules config.NormalizeRules) (string, error) {
	out := name

	if best := longestMatch(rules.StripPrefixes, func(p string) bool {
		return strings.HasPrefix(out, p)
	}); best != "" {
		out = strings.TrimPrefix(out, best)
	}
	if best := longestMatch(rules.StripSuffixes, func(s string) bool {
		return strings.HasSuffix(out, s)
	}); best != "" {
		out = strings.TrimSuffix(out, best)
	}

	if out == "" {
		return "", fmt.Errorf("name %q normalizes to an empty string; remove the affix rule that consumes it entirely", name)
	}
	return out, nil
}

// longestMatch returns the longest candidate satisfying match, or "".
func longestMatch(candidates []string, match func(string) bool) string {
	best := ""
	for _, c := range candidates {
		if match(c) && len(c) > len(best) {
			best = c
		}
	}
	return best
}

// key identifies a pair slot. Kind is part of the key so that a Deployment and
// a StatefulSet sharing a name remain distinct resources.
type key struct {
	Kind model.Kind
	Name string
}
