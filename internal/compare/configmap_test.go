package compare

import (
	"strings"
	"testing"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

func cm(name string, data map[string]string) model.ConfigMap {
	return model.ConfigMap{Kind: model.KindConfigMap, Name: name, Data: data}
}

// Masked ConfigMap entries carry the same placeholder text on both sides, so
// equality must be decided by fingerprint, never by the placeholder string —
// otherwise a rotated credential in a ConfigMap silently vanishes.
func TestConfigMapMaskedKeyDiffersByFingerprint(t *testing.T) {
	a := model.ConfigMap{Kind: model.KindConfigMap, Name: "app-config",
		Data:         map[string]string{"CLIENT_SECRET": "<redacted>"},
		Fingerprints: map[string]string{"CLIENT_SECRET": "fp-staging"},
	}
	b := model.ConfigMap{Kind: model.KindConfigMap, Name: "app-config",
		Data:         map[string]string{"CLIENT_SECRET": "<redacted>"},
		Fingerprints: map[string]string{"CLIENT_SECRET": "fp-prod"},
	}

	diffs := ConfigMap(Pair{Kind: model.KindConfigMap, Name: "app-config", FromCM: &a, ToCM: &b})
	d := find(diffs, "data.CLIENT_SECRET")
	if d == nil {
		t.Fatalf("differing fingerprints should be reported as drift, got %+v", diffs)
	}
	if !d.Redacted {
		t.Error("masked key diff should be flagged Redacted")
	}
	if d.From != "<redacted>" || d.To != "<redacted>" {
		t.Errorf("masked diff must show only the placeholder, got %q -> %q", d.From, d.To)
	}
}

// The same fingerprint on both sides means the underlying secret did not
// change, so no diff should be reported even though we cannot see the value.
func TestConfigMapMaskedKeySameFingerprintNoDiff(t *testing.T) {
	a := model.ConfigMap{Kind: model.KindConfigMap, Name: "app-config",
		Data:         map[string]string{"CLIENT_SECRET": "<redacted>"},
		Fingerprints: map[string]string{"CLIENT_SECRET": "fp-same"},
	}
	b := model.ConfigMap{Kind: model.KindConfigMap, Name: "app-config",
		Data:         map[string]string{"CLIENT_SECRET": "<redacted>"},
		Fingerprints: map[string]string{"CLIENT_SECRET": "fp-same"},
	}

	if diffs := ConfigMap(Pair{Kind: model.KindConfigMap, Name: "app-config", FromCM: &a, ToCM: &b}); len(diffs) != 0 {
		t.Errorf("equal fingerprints should produce no diff, got %+v", diffs)
	}
}

func TestConfigMapKeyDiffs(t *testing.T) {
	a := cm("app-config", map[string]string{"LOG_LEVEL": "debug", "ONLY_FROM": "1"})
	b := cm("app-config", map[string]string{"LOG_LEVEL": "info", "ONLY_TO": "2"})

	diffs := ConfigMap(Pair{Kind: model.KindConfigMap, Name: "app-config", FromCM: &a, ToCM: &b})

	cases := []struct {
		path     string
		typ      model.DiffType
		from, to string
	}{
		{"data.LOG_LEVEL", model.ValueChanged, "debug", "info"},
		{"data.ONLY_FROM", model.MissingInTo, "1", ""},
		{"data.ONLY_TO", model.MissingInFrom, "", "2"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			d := find(diffs, c.path)
			if d == nil {
				t.Fatalf("missing %q in %+v", c.path, diffs)
			}
			if d.Type != c.typ || d.From != c.from || d.To != c.to {
				t.Errorf("got {%s %q→%q}", d.Type, d.From, d.To)
			}
		})
	}
}

func TestConfigMapIdentical(t *testing.T) {
	a := cm("app-config", map[string]string{"K": "v"})
	b := cm("app-config", map[string]string{"K": "v"})
	if diffs := ConfigMap(Pair{Kind: model.KindConfigMap, Name: "app-config", FromCM: &a, ToCM: &b}); len(diffs) != 0 {
		t.Errorf("got %+v", diffs)
	}
}

func TestConfigMapMissingEntirely(t *testing.T) {
	a := cm("app-config", map[string]string{"K": "v"})
	diffs := ConfigMap(Pair{Kind: model.KindConfigMap, Name: "app-config", FromName: "app-config", FromCM: &a})
	if len(diffs) != 1 || diffs[0].Type != model.MissingInTo || diffs[0].Path != "" {
		t.Fatalf("got %+v", diffs)
	}
}

// An embedded config file must produce a line-level diff, not two blobs.
func TestConfigMapMultilineValue(t *testing.T) {
	from := "server:\n  port: 8080\n  timeout: 30\nlogging: debug\n"
	to := "server:\n  port: 8080\n  timeout: 60\nlogging: debug\n"
	a := cm("app-config", map[string]string{"application.yml": from})
	b := cm("app-config", map[string]string{"application.yml": to})

	diffs := ConfigMap(Pair{Kind: model.KindConfigMap, Name: "app-config", FromCM: &a, ToCM: &b})
	d := find(diffs, "data.application.yml")
	if d == nil {
		t.Fatalf("no diff for multi-line key: %+v", diffs)
	}
	if !d.Multiline {
		t.Error("Multiline flag should be set")
	}
	// Only the changed line is reported, not the whole file.
	if !strings.Contains(d.From, "timeout: 30") || !strings.Contains(d.To, "timeout: 60") {
		t.Errorf("changed line missing: from=%q to=%q", d.From, d.To)
	}
	if strings.Contains(d.From, "logging: debug") {
		t.Errorf("unchanged lines should be omitted, got %q", d.From)
	}
}

func TestDiffAllOrdersMissingBeforeChanges(t *testing.T) {
	a := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 3,
		Containers: []model.Container{{Name: "api", Env: map[string]model.EnvValue{
			"A": {Kind: model.EnvInline, Value: "1"},
			"B": {Kind: model.EnvInline, Value: "2"},
		}}}}
	b := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
		Containers: []model.Container{{Name: "api", Env: map[string]model.EnvValue{
			"A": {Kind: model.EnvInline, Value: "9"},
		}}}}

	diffs := DiffAll([]Pair{{Kind: model.KindDeployment, Name: "api", From: &a, To: &b}})
	if len(diffs) == 0 {
		t.Fatal("expected differences")
	}
	if diffs[0].Type == model.ValueChanged {
		t.Errorf("missing entries must sort before value changes, got %s first", diffs[0].Path)
	}
}

// Two runs over the same input must produce byte-identical ordering.
func TestDiffAllIsDeterministic(t *testing.T) {
	build := func() []Pair {
		a := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
			Containers: []model.Container{{Name: "api", Env: map[string]model.EnvValue{
				"Z": {Kind: model.EnvInline, Value: "1"},
				"A": {Kind: model.EnvInline, Value: "1"},
				"M": {Kind: model.EnvInline, Value: "1"},
			}}}}
		b := model.Workload{Kind: model.KindDeployment, Name: "api", Replicas: 1,
			Containers: []model.Container{{Name: "api", Env: map[string]model.EnvValue{
				"Z": {Kind: model.EnvInline, Value: "2"},
				"A": {Kind: model.EnvInline, Value: "2"},
				"M": {Kind: model.EnvInline, Value: "2"},
			}}}}
		return []Pair{{Kind: model.KindDeployment, Name: "api", From: &a, To: &b}}
	}
	first := DiffAll(build())
	for i := 0; i < 20; i++ {
		next := DiffAll(build())
		for j := range first {
			if first[j].Path != next[j].Path {
				t.Fatalf("nondeterministic ordering at %d: %q vs %q", j, first[j].Path, next[j].Path)
			}
		}
	}
}
