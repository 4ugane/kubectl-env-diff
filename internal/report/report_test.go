package report

import (
	"testing"

	"github.com/4ugane/kubectl-env-diff/internal/compare"
	"github.com/4ugane/kubectl-env-diff/internal/model"
)

func drift(name, path string) model.Difference {
	return model.Difference{Kind: model.KindDeployment, Name: name, Path: path,
		Type: model.ValueChanged, Severity: model.SeverityDrift, From: "a", To: "b"}
}

func TestBuildCounts(t *testing.T) {
	pairs := []compare.Pair{
		{Kind: model.KindDeployment, Name: "paired-clean"},
		{Kind: model.KindDeployment, Name: "paired-drifted"},
		{Kind: model.KindDeployment, Name: "gone"},
		{Kind: model.KindDeployment, Name: "added"},
	}
	// Mark which pairs have both sides so Build can count them.
	w := model.Workload{Kind: model.KindDeployment, Replicas: 1}
	pairs[0].From, pairs[0].To = &w, &w
	pairs[1].From, pairs[1].To = &w, &w
	pairs[2].From = &w
	pairs[3].To = &w

	diffs := []model.Difference{
		drift("paired-drifted", "replicas"),
		{Kind: model.KindDeployment, Name: "gone", Type: model.MissingInTo, Severity: model.SeverityDrift},
		{Kind: model.KindDeployment, Name: "added", Type: model.MissingInFrom, Severity: model.SeverityDrift},
		{Kind: model.KindDeployment, Name: "paired-clean", Path: "container[api].image.tag",
			Type: model.ValueChanged, Severity: model.SeverityExpected, From: "v1", To: "v2"},
	}

	r := Build(Meta{FromContext: "staging", FromNamespace: "web",
		ToContext: "prod", ToNamespace: "web"}, pairs, diffs, nil)

	if r.Summary.Paired != 2 {
		t.Errorf("Paired = %d, want 2", r.Summary.Paired)
	}
	if r.Summary.Drifted != 1 {
		t.Errorf("Drifted = %d, want 1", r.Summary.Drifted)
	}
	if r.Summary.Identical != 1 {
		t.Errorf("Identical = %d, want 1 (expected-only differences do not count as drift)", r.Summary.Identical)
	}
	if r.Summary.MissingInTo != 1 || r.Summary.MissingInFrom != 1 {
		t.Errorf("missing counts = %d/%d, want 1/1", r.Summary.MissingInTo, r.Summary.MissingInFrom)
	}
	if r.Summary.ExpectedHidden != 1 {
		t.Errorf("ExpectedHidden = %d, want 1", r.Summary.ExpectedHidden)
	}
}

func TestHasDrift(t *testing.T) {
	withDrift := Report{Differences: []model.Difference{drift("api", "replicas")}}
	if !withDrift.HasDrift() {
		t.Error("report with drift should report HasDrift")
	}

	expectedOnly := Report{Differences: []model.Difference{{
		Kind: model.KindDeployment, Name: "api", Path: "replicas",
		Type: model.ValueChanged, Severity: model.SeverityExpected}}}
	if expectedOnly.HasDrift() {
		t.Error("expected-only differences must not count as drift")
	}

	if (Report{}).HasDrift() {
		t.Error("empty report should not report drift")
	}
}

func TestDriftOnlyFiltersExpected(t *testing.T) {
	r := Report{Differences: []model.Difference{
		drift("api", "replicas"),
		{Kind: model.KindDeployment, Name: "api", Path: "container[api].image.tag",
			Type: model.ValueChanged, Severity: model.SeverityExpected},
	}}
	got := r.DriftOnly()
	if len(got) != 1 || got[0].Severity != model.SeverityDrift {
		t.Fatalf("got %+v", got)
	}
}

// A skipped kind means the report is incomplete and must say so.
func TestIncompleteWhenKindSkipped(t *testing.T) {
	r := Build(Meta{}, nil, nil, []model.SkipNote{{Kind: model.KindStatefulSet, Reason: "forbidden"}})
	if !r.Incomplete() {
		t.Error("a skipped kind must mark the report incomplete")
	}
	if r.Incomplete() != (len(r.Summary.Skipped) > 0) {
		t.Error("Incomplete must derive from Skipped")
	}
}

func TestGroupByResource(t *testing.T) {
	r := Report{Differences: []model.Difference{
		drift("api", "replicas"),
		drift("api", "serviceAccount"),
		drift("worker", "replicas"),
	}}
	groups := r.GroupByResource()
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if groups[0].Name != "api" || len(groups[0].Differences) != 2 {
		t.Errorf("first group = %+v", groups[0])
	}
	if groups[1].Name != "worker" || len(groups[1].Differences) != 1 {
		t.Errorf("second group = %+v", groups[1])
	}
}
