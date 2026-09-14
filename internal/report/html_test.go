package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

func TestHTMLSelfContained(t *testing.T) {
	var buf bytes.Buffer
	if err := HTML(&buf, sampleReport(), false); err != nil {
		t.Fatalf("HTML() error = %v", err)
	}
	out := buf.String()

	if !strings.HasPrefix(strings.TrimSpace(out), "<!DOCTYPE html>") {
		t.Error("output should be a complete HTML document")
	}
	// No external assets: the report must open from a Jira attachment offline.
	for _, bad := range []string{"http://", "https://", "//cdn"} {
		if strings.Contains(out, bad) {
			t.Errorf("external reference %q makes the report non-portable", bad)
		}
	}
	if !strings.Contains(out, "prefers-color-scheme") {
		t.Error("template should support dark mode")
	}
}

// Env values and ConfigMap contents are untrusted input.
func TestHTMLEscapesUntrustedValues(t *testing.T) {
	r := Report{
		Summary: Summary{Meta: Meta{FromContext: "a", ToContext: "b"}, Paired: 1, Drifted: 1},
		Differences: []model.Difference{{
			Kind: model.KindConfigMap, Name: "app-config", Path: "data.SNIPPET",
			Type: model.ValueChanged, Severity: model.SeverityDrift,
			From: "<script>alert(1)</script>", To: "safe",
		}},
	}
	var buf bytes.Buffer
	if err := HTML(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "<script>alert(1)</script>") {
		t.Fatal("unescaped markup from a ConfigMap value reached the output")
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("value should appear HTML-escaped")
	}
}

// One row of heatmap is noise, so it is suppressed below three resources.
func TestHeatmapSuppressedBelowThreshold(t *testing.T) {
	two := Report{
		Summary: Summary{Meta: Meta{FromContext: "a", ToContext: "b"}, Paired: 2, Drifted: 2},
		Differences: []model.Difference{
			{Kind: model.KindDeployment, Name: "api", Path: "replicas",
				Type: model.ValueChanged, Severity: model.SeverityDrift},
			{Kind: model.KindDeployment, Name: "worker", Path: "replicas",
				Type: model.ValueChanged, Severity: model.SeverityDrift},
		},
	}
	if Heatmap(two) != nil {
		t.Error("heatmap should be suppressed with only 2 drifted resources")
	}

	three := two
	three.Differences = append(append([]model.Difference{}, two.Differences...),
		model.Difference{Kind: model.KindDeployment, Name: "cron", Path: "replicas",
			Type: model.ValueChanged, Severity: model.SeverityDrift})
	if Heatmap(three) == nil {
		t.Error("heatmap should render with 3 drifted resources")
	}
}

func TestHeatmapCounts(t *testing.T) {
	r := Report{
		Summary: Summary{Meta: Meta{FromContext: "a", ToContext: "b"}},
		Differences: []model.Difference{
			{Kind: model.KindDeployment, Name: "api", Path: "container[api].resources.limits.memory",
				Type: model.ValueChanged, Severity: model.SeverityDrift},
			{Kind: model.KindDeployment, Name: "api", Path: "container[api].env.LOG_LEVEL",
				Type: model.ValueChanged, Severity: model.SeverityDrift},
			{Kind: model.KindDeployment, Name: "worker", Path: "container[w].resources.limits.cpu",
				Type: model.ValueChanged, Severity: model.SeverityDrift},
			{Kind: model.KindDeployment, Name: "cron", Path: "container[c].resources.limits.cpu",
				Type: model.ValueChanged, Severity: model.SeverityDrift},
		},
	}
	h := Heatmap(r)
	if h == nil {
		t.Fatal("heatmap should render")
	}
	if len(h.Rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(h.Rows))
	}
	// Rows are sorted by (Kind, Name) alphabetically, matching every other
	// sort in this codebase (compare.Sort, sortPairs, GroupByResource) — not
	// by total count. All three rows share Kind=Deployment here, so the
	// tiebreaker is name: "api" < "cron" < "worker" puts api first.
	if h.Rows[0].Name != "api" {
		t.Errorf("rows not sorted: first = %q", h.Rows[0].Name)
	}

	var api *HeatRow
	for i := range h.Rows {
		if h.Rows[i].Name == "api" {
			api = &h.Rows[i]
		}
	}
	if api == nil {
		t.Fatal("api row missing")
	}
	if api.Counts["resources"] != 1 || api.Counts["env"] != 1 {
		t.Errorf("api counts = %v", api.Counts)
	}
	// The systemic finding: resources drift across all three resources.
	if h.CategoryTotals["resources"] != 3 {
		t.Errorf("resources total = %d, want 3", h.CategoryTotals["resources"])
	}
}

func TestHTMLIncludesHeatmapWhenEligible(t *testing.T) {
	r := sampleReport()
	r.Differences = append(r.Differences,
		model.Difference{Kind: model.KindDeployment, Name: "worker", Path: "serviceAccount",
			Type: model.ValueChanged, Severity: model.SeverityDrift, From: "a", To: "b"},
		model.Difference{Kind: model.KindDeployment, Name: "cron", Path: "serviceAccount",
			Type: model.ValueChanged, Severity: model.SeverityDrift, From: "a", To: "b"})

	var buf bytes.Buffer
	if err := HTML(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "heat") {
		t.Error("heatmap section missing")
	}
}

// Badges show a short human word, not the raw Go type name, so a reader never
// has to decode "MissingInFrom" to know a key was added.
func TestHTMLBadgesUseHumanLabels(t *testing.T) {
	r := Report{
		Summary: Summary{Meta: Meta{FromContext: "a", ToContext: "b"}, Paired: 1, Drifted: 1},
		Differences: []model.Difference{
			{Kind: model.KindConfigMap, Name: "cfg", Path: "data.NEW_KEY",
				Type: model.MissingInFrom, Severity: model.SeverityDrift, To: "v"},
			{Kind: model.KindConfigMap, Name: "cfg", Path: "data.OLD_KEY",
				Type: model.MissingInTo, Severity: model.SeverityDrift, From: "v"},
			{Kind: model.KindConfigMap, Name: "cfg", Path: "data.CHANGED_KEY",
				Type: model.ValueChanged, Severity: model.SeverityDrift, From: "a", To: "b"},
		},
	}
	var buf bytes.Buffer
	if err := HTML(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, raw := range []string{"MissingInFrom", "MissingInTo", "ValueChanged"} {
		if strings.Contains(out, raw) {
			t.Errorf("raw type name %q leaked into badge text, want a human label", raw)
		}
	}
	for _, label := range []string{"added", "removed", "changed"} {
		if !strings.Contains(out, label) {
			t.Errorf("expected human label %q in output", label)
		}
	}
}

func TestHTMLSkippedWarning(t *testing.T) {
	r := Report{Summary: Summary{
		Meta:    Meta{FromContext: "staging", ToContext: "prod"},
		Skipped: []model.SkipNote{{Kind: model.KindStatefulSet, Reason: "forbidden"}},
	}}
	var buf bytes.Buffer
	if err := HTML(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "incomplete") {
		t.Error("skipped kinds must be called out in HTML too")
	}
}
