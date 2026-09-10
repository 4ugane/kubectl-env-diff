// Package report assembles summary counts and renders results.
//
// Renderers in this package are pure formatters over Report. Redaction,
// classification, and ignore rules have already been applied upstream, so no
// renderer can reach a raw value — which is what makes four output formats
// cheap and keeps secret safety independent of the renderer count.
package report

import (
	"sort"

	"github.com/4ugane/kubectl-env-diff/internal/compare"
	"github.com/4ugane/kubectl-env-diff/internal/model"
)

// Meta identifies the two sides being compared.
type Meta struct {
	FromContext   string `json:"fromContext"`
	FromNamespace string `json:"fromNamespace"`
	ToContext     string `json:"toContext"`
	ToNamespace   string `json:"toNamespace"`
}

// Summary is the headline count block.
type Summary struct {
	Meta
	Pairs          int              `json:"pairs"`
	Paired         int              `json:"paired"`
	Identical      int              `json:"identical"`
	Drifted        int              `json:"drifted"`
	MissingInFrom  int              `json:"missingInFrom"`
	MissingInTo    int              `json:"missingInTo"`
	ExpectedHidden int              `json:"expectedHidden"`
	Skipped        []model.SkipNote `json:"skipped,omitempty"`
}

// Report is the complete result, and the only input a renderer receives.
type Report struct {
	Summary     Summary            `json:"summary"`
	Differences []model.Difference `json:"differences"`
}

// Group collects the differences for one resource, for renderers that display
// per-resource sections.
type Group struct {
	Kind        model.Kind
	Name        string
	Differences []model.Difference
}

// Build computes summary counts from pairs and already-classified differences.
func Build(meta Meta, pairs []compare.Pair, diffs []model.Difference, skipped []model.SkipNote) Report {
	s := Summary{Meta: meta, Pairs: len(pairs), Skipped: skipped}

	for _, p := range pairs {
		bothSides := (p.From != nil && p.To != nil) || (p.FromCM != nil && p.ToCM != nil)
		if bothSides {
			s.Paired++
		}
	}

	// A resource counts as drifted if it has at least one Drift difference.
	// Expected-only differences leave it identical, matching what the summary
	// line claims.
	driftedResources := map[string]bool{}
	for _, d := range diffs {
		switch d.Severity {
		case model.SeverityExpected:
			s.ExpectedHidden++
			continue
		}
		switch d.Type {
		case model.MissingInTo:
			if d.Path == "" {
				s.MissingInTo++
				continue
			}
		case model.MissingInFrom:
			if d.Path == "" {
				s.MissingInFrom++
				continue
			}
		}
		driftedResources[string(d.Kind)+"/"+d.Name] = true
	}
	s.Drifted = len(driftedResources)
	s.Identical = s.Paired - s.Drifted
	if s.Identical < 0 {
		s.Identical = 0
	}

	return Report{Summary: s, Differences: diffs}
}

// HasDrift reports whether any difference is drift rather than expected. This
// is the sole input to the drift exit code.
func (r Report) HasDrift() bool {
	for _, d := range r.Differences {
		if d.Severity == model.SeverityDrift {
			return true
		}
	}
	return false
}

// Incomplete reports whether any kind could not be read. An incomplete report
// must never be presented as a clean bill of health.
func (r Report) Incomplete() bool { return len(r.Summary.Skipped) > 0 }

// DriftOnly returns just the drift differences, preserving order.
func (r Report) DriftOnly() []model.Difference {
	out := make([]model.Difference, 0, len(r.Differences))
	for _, d := range r.Differences {
		if d.Severity == model.SeverityDrift {
			out = append(out, d)
		}
	}
	return out
}

// GroupByResource groups drift differences by resource, in the order the
// differences were already sorted into.
func (r Report) GroupByResource() []Group {
	index := map[string]int{}
	var groups []Group
	for _, d := range r.DriftOnly() {
		if d.Path == "" {
			continue // whole-resource absence is rendered in its own section
		}
		k := string(d.Kind) + "/" + d.Name
		i, ok := index[k]
		if !ok {
			groups = append(groups, Group{Kind: d.Kind, Name: d.Name})
			i = len(groups) - 1
			index[k] = i
		}
		groups[i].Differences = append(groups[i].Differences, d)
	}
	sort.SliceStable(groups, func(a, b int) bool {
		if groups[a].Kind != groups[b].Kind {
			return groups[a].Kind < groups[b].Kind
		}
		return groups[a].Name < groups[b].Name
	})
	return groups
}

// Missing returns whole-resource absences for the given direction.
func (r Report) Missing(typ model.DiffType) []model.Difference {
	var out []model.Difference
	for _, d := range r.Differences {
		if d.Type == typ && d.Path == "" && d.Severity == model.SeverityDrift {
			out = append(out, d)
		}
	}
	return out
}

// displayValues resolves the strings shown for a difference, substituting the
// missing marker and the redaction placeholders. Shared by every renderer so
// they cannot diverge on how a redacted difference is presented.
func displayValues(d model.Difference) (string, string) {
	if d.Redacted && d.Type == model.ValueChanged {
		return "<redacted>", "<redacted, differs>"
	}
	from, to := d.From, d.To
	if from == "" && d.Type == model.MissingInFrom {
		from = "(missing)"
	}
	if to == "" && d.Type == model.MissingInTo {
		to = "(missing)"
	}
	return from, to
}
