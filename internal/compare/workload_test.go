package compare

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

func rl(pairs map[string]string) model.ResourceList {
	if pairs == nil {
		return nil
	}
	out := model.ResourceList{}
	for k, v := range pairs {
		out[k] = resource.MustParse(v)
	}
	return out
}

func find(diffs []model.Difference, path string) *model.Difference {
	for i := range diffs {
		if diffs[i].Path == path {
			return &diffs[i]
		}
	}
	return nil
}

func TestWorkloadIdenticalProducesNoDiffs(t *testing.T) {
	a := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 2,
		Containers: []model.Container{{
			Name:   "api",
			Image:  model.ParseImage("acme/api:v1"),
			Env:    map[string]model.EnvValue{"LOG_LEVEL": {Kind: model.EnvInline, Value: "info"}},
			Limits: rl(map[string]string{"memory": "1Gi"}),
		}}}
	b := a
	if diffs := Workload(Pair{Kind: model.KindDeployment, Name: "api", From: &a, To: &b}); len(diffs) != 0 {
		t.Errorf("identical workloads produced %d diffs: %+v", len(diffs), diffs)
	}
}

// 1Gi and 1024Mi are the same amount and must not report drift.
func TestWorkloadEquivalentQuantities(t *testing.T) {
	a := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
		Containers: []model.Container{{Name: "api", Limits: rl(map[string]string{"memory": "1Gi"})}}}
	b := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
		Containers: []model.Container{{Name: "api", Limits: rl(map[string]string{"memory": "1024Mi"})}}}

	if diffs := Workload(Pair{Kind: model.KindDeployment, Name: "api", From: &a, To: &b}); len(diffs) != 0 {
		t.Errorf("1Gi vs 1024Mi should be equal, got %+v", diffs)
	}
}

func TestWorkloadValueChanges(t *testing.T) {
	a := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 3, ServiceAccount: "sa-a",
		Containers: []model.Container{{
			Name:  "api",
			Image: model.ParseImage("acme/api:v1"),
			Env: map[string]model.EnvValue{
				"LOG_LEVEL": {Kind: model.EnvInline, Value: "debug"},
				"ONLY_FROM": {Kind: model.EnvInline, Value: "x"},
			},
			Limits:    rl(map[string]string{"memory": "4Gi"}),
			Readiness: model.Probe{Present: true, Type: "httpGet", Target: "/healthz:8080"},
		}}}
	b := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1, ServiceAccount: "sa-b",
		Containers: []model.Container{{
			Name:  "api",
			Image: model.ParseImage("acme/api:v2"),
			Env: map[string]model.EnvValue{
				"LOG_LEVEL": {Kind: model.EnvInline, Value: "info"},
				"ONLY_TO":   {Kind: model.EnvInline, Value: "y"},
			},
			Limits:    rl(map[string]string{"memory": "512Mi"}),
			Readiness: model.Probe{Present: true, Type: "httpGet", Target: "/health:8080"},
		}}}

	diffs := Workload(Pair{Kind: model.KindDeployment, Name: "api", From: &a, To: &b})

	cases := []struct {
		path     string
		typ      model.DiffType
		from, to string
	}{
		{"replicas", model.ValueChanged, "3", "1"},
		{"serviceAccount", model.ValueChanged, "sa-a", "sa-b"},
		{"container[api].image.tag", model.ValueChanged, "v1", "v2"},
		{"container[api].env.LOG_LEVEL", model.ValueChanged, "debug", "info"},
		{"container[api].env.ONLY_FROM", model.MissingInTo, "x", ""},
		{"container[api].env.ONLY_TO", model.MissingInFrom, "", "y"},
		{"container[api].resources.limits.memory", model.ValueChanged, "4Gi", "512Mi"},
		{"container[api].probes.readiness.target", model.ValueChanged, "/healthz:8080", "/health:8080"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			d := find(diffs, c.path)
			if d == nil {
				t.Fatalf("no difference at %q; got %+v", c.path, diffs)
			}
			if d.Type != c.typ || d.From != c.from || d.To != c.to {
				t.Errorf("got {%s %q→%q}, want {%s %q→%q}", d.Type, d.From, d.To, c.typ, c.from, c.to)
			}
		})
	}

	// image.repo is unchanged, so it must not appear.
	if d := find(diffs, "container[api].image.repo"); d != nil {
		t.Errorf("unchanged repo should not be reported: %+v", d)
	}
}

// A whole workload absent on one side is the headline finding.
func TestWorkloadMissingEntirely(t *testing.T) {
	a := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1}

	diffs := Workload(Pair{Kind: model.KindDeployment, Name: "api", FromName: "api", From: &a})
	if len(diffs) != 1 {
		t.Fatalf("got %d diffs, want 1", len(diffs))
	}
	if diffs[0].Type != model.MissingInTo || diffs[0].Path != "" {
		t.Errorf("got %+v", diffs[0])
	}

	diffs = Workload(Pair{Kind: model.KindDeployment, Name: "api", ToName: "api", To: &a})
	if len(diffs) != 1 || diffs[0].Type != model.MissingInFrom {
		t.Fatalf("got %+v", diffs)
	}
}

// Container granularity: "container[worker] missing" beats "containers differ".
func TestWorkloadContainerMissing(t *testing.T) {
	a := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
		Containers: []model.Container{{Name: "api"}, {Name: "sidecar"}}}
	b := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
		Containers: []model.Container{{Name: "api"}}}

	diffs := Workload(Pair{Kind: model.KindDeployment, Name: "api", From: &a, To: &b})
	d := find(diffs, "container[sidecar]")
	if d == nil || d.Type != model.MissingInTo {
		t.Fatalf("expected container[sidecar] MissingInTo, got %+v", diffs)
	}
}

func TestWorkloadInitContainersDiffedSeparately(t *testing.T) {
	a := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
		InitContainers: []model.Container{{Name: "migrate", Image: model.ParseImage("acme/migrate:v1")}}}
	b := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
		InitContainers: []model.Container{{Name: "migrate", Image: model.ParseImage("acme/migrate:v2")}}}

	diffs := Workload(Pair{Kind: model.KindDeployment, Name: "api", From: &a, To: &b})
	if d := find(diffs, "initContainer[migrate].image.tag"); d == nil {
		t.Fatalf("init container drift missed: %+v", diffs)
	}
}

// A probe removed in one environment must not read as no change.
func TestWorkloadProbeAbsence(t *testing.T) {
	a := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
		Containers: []model.Container{{Name: "api",
			Liveness: model.Probe{Present: true, Type: "httpGet", Target: "/live:8080"}}}}
	b := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
		Containers: []model.Container{{Name: "api"}}}

	diffs := Workload(Pair{Kind: model.KindDeployment, Name: "api", From: &a, To: &b})
	d := find(diffs, "container[api].probes.liveness")
	if d == nil || d.Type != model.MissingInTo {
		t.Fatalf("removed probe not reported: %+v", diffs)
	}
}

// Redacted differences report that they differ, never what to.
func TestWorkloadRedactedValueNeverShown(t *testing.T) {
	a := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
		Containers: []model.Container{{Name: "api", Env: map[string]model.EnvValue{
			"DB_PASSWORD": {Kind: model.EnvInline, Value: "<redacted>", Redacted: true}}}}}
	b := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
		Containers: []model.Container{{Name: "api", Env: map[string]model.EnvValue{
			"DB_PASSWORD": {Kind: model.EnvInline, Value: "<redacted>", Redacted: true}}}}}

	// Two redacted values are indistinguishable, so no difference is emitted.
	if diffs := Workload(Pair{Kind: model.KindDeployment, Name: "api", From: &a, To: &b}); len(diffs) != 0 {
		t.Errorf("equal redacted values should produce no diff, got %+v", diffs)
	}
}

// A rotated credential must still be detected as drift, without its value
// ever appearing: two redacted env values with different fingerprints (the
// underlying secret differs) must be reported as changed, marked Redacted,
// and never show anything but the placeholder text.
func TestWorkloadRedactedValueDiffersByFingerprint(t *testing.T) {
	a := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
		Containers: []model.Container{{Name: "api", Env: map[string]model.EnvValue{
			"DB_PASSWORD": {Kind: model.EnvInline, Value: "<redacted>", Redacted: true, Fingerprint: "fp-staging"}}}}}
	b := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
		Containers: []model.Container{{Name: "api", Env: map[string]model.EnvValue{
			"DB_PASSWORD": {Kind: model.EnvInline, Value: "<redacted>", Redacted: true, Fingerprint: "fp-prod"}}}}}

	diffs := Workload(Pair{Kind: model.KindDeployment, Name: "api", From: &a, To: &b})
	d := find(diffs, "container[api].env.DB_PASSWORD")
	if d == nil {
		t.Fatalf("differing fingerprints should be reported as drift, got %+v", diffs)
	}
	if !d.Redacted {
		t.Error("differing redacted value should be flagged Redacted")
	}
	if d.From != "<redacted>" || d.To != "<redacted>" {
		t.Errorf("redacted diff must show only the placeholder, got %q -> %q", d.From, d.To)
	}
}
