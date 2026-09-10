package compare

import (
	"sort"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

// DiffAll diffs every pair and returns differences in a total order.
//
// The ordering is deliberate and load-bearing: missing entries sort before
// value changes because an absent key or workload is the classic cause of an
// environment-specific failure and should lead the report. The order is also
// fully determined, so two runs over identical input produce identical output
// and golden tests do not flake.
func DiffAll(pairs []Pair) []model.Difference {
	var out []model.Difference
	for _, p := range pairs {
		if p.Kind == model.KindConfigMap {
			out = append(out, ConfigMap(p)...)
			continue
		}
		out = append(out, Workload(p)...)
	}
	Sort(out)
	return out
}

// typeRank orders difference types. Lower sorts first.
func typeRank(t model.DiffType) int {
	switch t {
	case model.MissingInTo:
		return 0
	case model.MissingInFrom:
		return 1
	default:
		return 2
	}
}

// Sort imposes the total order in place.
func Sort(diffs []model.Difference) {
	sort.SliceStable(diffs, func(i, j int) bool {
		a, b := diffs[i], diffs[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if ra, rb := typeRank(a.Type), typeRank(b.Type); ra != rb {
			return ra < rb
		}
		return a.Path < b.Path
	})
}
