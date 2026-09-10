package classify

import (
	"testing"

	"github.com/4ugane/kubectl-env-diff/internal/config"
	"github.com/4ugane/kubectl-env-diff/internal/model"
)

func diff(path string, typ model.DiffType) model.Difference {
	return model.Difference{
		Kind: model.KindDeployment, Name: "api", Path: path, Type: typ,
		From: "a", To: "b",
	}
}

func TestExpectedSetIsMinimal(t *testing.T) {
	tests := []struct {
		path string
		want model.Severity
	}{
		// The entire built-in Expected set.
		{"replicas", model.SeverityExpected},
		{"container[api].image.tag", model.SeverityExpected},
		{"initContainer[migrate].image.tag", model.SeverityExpected},

		// Everything else is drift, including things a cleverer tool might
		// try to auto-excuse.
		{"container[api].image.repo", model.SeverityDrift},
		{"container[api].image.digest", model.SeverityDrift},
		{"container[api].resources.limits.memory", model.SeverityDrift},
		{"container[api].env.LOG_LEVEL", model.SeverityDrift},
		{"container[api].env.AWS_ACCOUNT_ID", model.SeverityDrift},
		{"container[api].env.API_HOSTNAME", model.SeverityDrift},
		{"container[api].probes.readiness.target", model.SeverityDrift},
		{"serviceAccount", model.SeverityDrift},
		{"data.LOG_LEVEL", model.SeverityDrift},
		{"", model.SeverityDrift},
	}
	for _, tc := range tests {
		name := tc.path
		if name == "" {
			name = "(whole workload)"
		}
		t.Run(name, func(t *testing.T) {
			got := Apply([]model.Difference{diff(tc.path, model.ValueChanged)}, nil)
			if len(got) != 1 {
				t.Fatalf("got %d differences, want 1", len(got))
			}
			if got[0].Severity != tc.want {
				t.Errorf("severity = %q, want %q", got[0].Severity, tc.want)
			}
		})
	}
}

// A missing image tag is not routine — the image reference is absent, which is
// a real problem, not an expected environment difference.
func TestMissingImageTagIsDrift(t *testing.T) {
	got := Apply([]model.Difference{diff("container[api].image.tag", model.MissingInTo)}, nil)
	if got[0].Severity != model.SeverityDrift {
		t.Errorf("severity = %q, want Drift", got[0].Severity)
	}
}

func TestIgnoreByPath(t *testing.T) {
	diffs := []model.Difference{
		diff("container[api].env.DATADOG_ENV", model.ValueChanged),
		diff("container[api].env.LOG_LEVEL", model.ValueChanged),
	}
	rules := []config.IgnoreRule{{Path: "container[*].env.DATADOG_ENV"}}

	got := Apply(diffs, rules)
	if len(got) != 1 {
		t.Fatalf("got %d differences, want 1", len(got))
	}
	if got[0].Path != "container[api].env.LOG_LEVEL" {
		t.Errorf("wrong difference survived: %q", got[0].Path)
	}
}

func TestIgnoreByKindAndName(t *testing.T) {
	diffs := []model.Difference{
		{Kind: model.KindConfigMap, Name: "aws-auth", Path: "data.mapRoles", Type: model.ValueChanged},
		{Kind: model.KindConfigMap, Name: "app-config", Path: "data.LOG_LEVEL", Type: model.ValueChanged},
	}
	rules := []config.IgnoreRule{{Kind: "ConfigMap", Name: "aws-auth"}}

	got := Apply(diffs, rules)
	if len(got) != 1 || got[0].Name != "app-config" {
		t.Fatalf("got %+v", got)
	}
}

// A rule with several fields set must match ALL of them, not any.
func TestIgnoreRuleFieldsAreConjunctive(t *testing.T) {
	diffs := []model.Difference{
		{Kind: model.KindDeployment, Name: "api", Path: "replicas", Type: model.ValueChanged},
	}
	// Path matches but kind does not, so the rule must not apply.
	rules := []config.IgnoreRule{{Kind: "ConfigMap", Path: "replicas"}}

	if got := Apply(diffs, rules); len(got) != 1 {
		t.Errorf("rule should not match when kind differs, got %d differences", len(got))
	}
}

func TestApplyPreservesOrder(t *testing.T) {
	diffs := []model.Difference{
		diff("a", model.ValueChanged), diff("b", model.ValueChanged), diff("c", model.ValueChanged),
	}
	got := Apply(diffs, nil)
	for i, want := range []string{"a", "b", "c"} {
		if got[i].Path != want {
			t.Fatalf("order changed: got %q at %d, want %q", got[i].Path, i, want)
		}
	}
}

func TestApplyDoesNotMutateInput(t *testing.T) {
	diffs := []model.Difference{diff("replicas", model.ValueChanged)}
	_ = Apply(diffs, nil)
	if diffs[0].Severity != "" {
		t.Error("Apply must not mutate its input")
	}
}
