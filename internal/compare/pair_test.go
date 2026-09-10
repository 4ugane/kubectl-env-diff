package compare

import (
	"strings"
	"testing"

	"github.com/4ugane/kubectl-env-diff/internal/config"
	"github.com/4ugane/kubectl-env-diff/internal/model"
)

func wl(kind model.Kind, name string) model.Workload {
	return model.Workload{Kind: kind, Name: name, Replicas: 1}
}

func TestPairByNormalizedName(t *testing.T) {
	from := model.Snapshot{Workloads: []model.Workload{
		wl(model.KindDeployment, "api-staging"),
		wl(model.KindDeployment, "orphan-staging"),
	}}
	to := model.Snapshot{Workloads: []model.Workload{
		wl(model.KindDeployment, "api-prod"),
		wl(model.KindDeployment, "newcomer-prod"),
	}}
	opts := Options{Normalize: config.NormalizeRules{StripSuffixes: []string{"-staging", "-prod"}}}

	pairs, err := Pairs(from, to, opts)
	if err != nil {
		t.Fatalf("Pairs() error = %v", err)
	}
	if len(pairs) != 3 {
		t.Fatalf("got %d pairs, want 3 (api, orphan, newcomer)", len(pairs))
	}

	byName := map[string]Pair{}
	for _, p := range pairs {
		byName[p.Name] = p
	}
	if p := byName["api"]; p.From == nil || p.To == nil {
		t.Error("api should be paired on both sides")
	}
	if p := byName["orphan"]; p.From == nil || p.To != nil {
		t.Error("orphan should exist only on the from side")
	}
	if p := byName["newcomer"]; p.From != nil || p.To == nil {
		t.Error("newcomer should exist only on the to side")
	}
}

// A Deployment and a StatefulSet with the same name are different resources.
func TestPairKeyedByKindAndName(t *testing.T) {
	from := model.Snapshot{Workloads: []model.Workload{
		wl(model.KindDeployment, "worker"),
		wl(model.KindStatefulSet, "worker"),
	}}
	to := model.Snapshot{Workloads: []model.Workload{
		wl(model.KindDeployment, "worker"),
		wl(model.KindStatefulSet, "worker"),
	}}
	pairs, err := Pairs(from, to, Options{})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want 2 (one per kind)", len(pairs))
	}
	for _, p := range pairs {
		if p.From == nil || p.To == nil {
			t.Errorf("%s/%s should be paired", p.Kind, p.Name)
		}
	}
}

// Two names collapsing to one is ambiguous. Guessing would produce a
// confident, fictional diff, so it must fail.
func TestPairRejectsNormalizationCollision(t *testing.T) {
	from := model.Snapshot{Workloads: []model.Workload{
		wl(model.KindDeployment, "api"),
		wl(model.KindDeployment, "api-staging"),
	}}
	to := model.Snapshot{Workloads: []model.Workload{wl(model.KindDeployment, "api")}}
	opts := Options{Normalize: config.NormalizeRules{StripSuffixes: []string{"-staging"}}}

	_, err := Pairs(from, to, opts)
	if err == nil {
		t.Fatal("collision should be a hard error")
	}
	for _, want := range []string{"api", "api-staging"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should name offender %q, got %v", want, err)
		}
	}
}

// Explicit names bypass normalization entirely.
func TestPairExplicitNames(t *testing.T) {
	from := model.Snapshot{Workloads: []model.Workload{wl(model.KindDeployment, "test-stagin")}}
	to := model.Snapshot{Workloads: []model.Workload{wl(model.KindDeployment, "test-prod")}}
	opts := Options{FromName: "test-stagin", ToName: "test-prod"}

	pairs, err := Pairs(from, to, opts)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(pairs) != 1 {
		t.Fatalf("got %d pairs, want 1", len(pairs))
	}
	if pairs[0].From == nil || pairs[0].To == nil {
		t.Fatal("explicit pair should match both sides")
	}
	if pairs[0].FromName != "test-stagin" || pairs[0].ToName != "test-prod" {
		t.Errorf("pair names = %q/%q", pairs[0].FromName, pairs[0].ToName)
	}
}

// Absent on both sides is a typo, not a finding.
func TestPairExplicitMissingBothSidesIsError(t *testing.T) {
	from := model.Snapshot{Workloads: []model.Workload{wl(model.KindDeployment, "api")}}
	to := model.Snapshot{Workloads: []model.Workload{wl(model.KindDeployment, "api")}}
	opts := Options{FromName: "typo-a", ToName: "typo-b"}

	_, err := Pairs(from, to, opts)
	if err == nil {
		t.Fatal("names absent from both sides should error, not report a finding")
	}
	if !strings.Contains(err.Error(), "typo-a") {
		t.Errorf("error should name the missing resource, got %v", err)
	}
}

// Absent on one side IS a finding — that is the headline result.
func TestPairExplicitMissingOneSideIsFinding(t *testing.T) {
	from := model.Snapshot{Workloads: []model.Workload{wl(model.KindDeployment, "test-stagin")}}
	to := model.Snapshot{}
	opts := Options{FromName: "test-stagin", ToName: "test-prod"}

	pairs, err := Pairs(from, to, opts)
	if err != nil {
		t.Fatalf("one-sided explicit pair should be a finding, got error %v", err)
	}
	if len(pairs) != 1 || pairs[0].From == nil || pairs[0].To != nil {
		t.Fatalf("pairs = %+v", pairs)
	}
}

// --name applies the same name to both sides, with normalization.
func TestPairSingleName(t *testing.T) {
	from := model.Snapshot{
		Workloads:  []model.Workload{wl(model.KindDeployment, "api"), wl(model.KindDeployment, "other")},
		ConfigMaps: []model.ConfigMap{{Kind: model.KindConfigMap, Name: "api"}},
	}
	to := model.Snapshot{Workloads: []model.Workload{wl(model.KindDeployment, "api")}}
	opts := Options{Name: "api"}

	pairs, err := Pairs(from, to, opts)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	// "other" is filtered out; the ConfigMap named api is kept.
	if len(pairs) != 2 {
		t.Fatalf("got %d pairs, want 2 (Deployment/api and ConfigMap/api)", len(pairs))
	}
	for _, p := range pairs {
		if p.Name != "api" {
			t.Errorf("unexpected pair %s/%s", p.Kind, p.Name)
		}
	}
}

func TestPairIsSortedDeterministically(t *testing.T) {
	from := model.Snapshot{Workloads: []model.Workload{
		wl(model.KindStatefulSet, "zebra"), wl(model.KindDeployment, "beta"), wl(model.KindDeployment, "alpha"),
	}}
	pairs, err := Pairs(from, model.Snapshot{}, Options{})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	want := []string{"Deployment/alpha", "Deployment/beta", "StatefulSet/zebra"}
	for i, w := range want {
		got := string(pairs[i].Kind) + "/" + pairs[i].Name
		if got != w {
			t.Fatalf("pairs[%d] = %s, want %s", i, got, w)
		}
	}
}
