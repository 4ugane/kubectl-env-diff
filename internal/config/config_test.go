package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValid(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "valid.yaml"), true)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Normalize.StripSuffixes) != 2 {
		t.Errorf("got %d suffixes, want 2", len(cfg.Normalize.StripSuffixes))
	}
	if len(cfg.Ignore) != 3 {
		t.Fatalf("got %d ignore rules, want 3", len(cfg.Ignore))
	}
	if cfg.Ignore[2].Kind != "ConfigMap" || cfg.Ignore[2].Name != "aws-auth" {
		t.Errorf("third rule = %+v", cfg.Ignore[2])
	}
}

// A missing default config file is normal, not an error.
func TestLoadMissingDefaultIsOK(t *testing.T) {
	cfg, err := Load(filepath.Join("testdata", "does-not-exist.yaml"), false)
	if err != nil {
		t.Fatalf("missing default config should not error, got %v", err)
	}
	if len(cfg.Ignore) != 0 || len(cfg.Normalize.StripSuffixes) != 0 {
		t.Error("missing config should yield empty defaults")
	}
}

// A file the user explicitly named must exist.
func TestLoadMissingExplicitIsError(t *testing.T) {
	_, err := Load(filepath.Join("testdata", "does-not-exist.yaml"), true)
	if err == nil {
		t.Fatal("explicitly requested missing config should error")
	}
	if !strings.Contains(err.Error(), "does-not-exist.yaml") {
		t.Errorf("error should name the file, got %v", err)
	}
}

func TestLoadRejectsEmptySuffix(t *testing.T) {
	_, err := Load(filepath.Join("testdata", "bad-suffix.yaml"), true)
	if err == nil {
		t.Fatal("empty suffix should be rejected")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should explain the problem, got %v", err)
	}
}

func TestValidateRejectsEmptyIgnoreRule(t *testing.T) {
	cfg := &Config{Ignore: []IgnoreRule{{}}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("an ignore rule with no fields set should be rejected")
	}
}

func TestValidateRejectsUnknownKind(t *testing.T) {
	cfg := &Config{Ignore: []IgnoreRule{{Kind: "Pod", Name: "x"}}}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("unknown kind should be rejected")
	}
	if !strings.Contains(err.Error(), "Pod") {
		t.Errorf("error should name the bad kind, got %v", err)
	}
}

// Ignore paths contain literal square brackets, as in
// "container[*].env.FOO". filepath.Match would read "[*]" as a character
// class, so the matcher treats everything except "*" as literal.
func TestMatchPath(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"replicas", "replicas", true},
		{"replicas", "serviceAccount", false},
		{"container[*].env.DATADOG_ENV", "container[api].env.DATADOG_ENV", true},
		{"container[*].env.DATADOG_ENV", "container[sidecar].env.DATADOG_ENV", true},
		{"container[*].env.DATADOG_ENV", "container[api].env.LOG_LEVEL", false},
		{"container[api].*", "container[api].image.tag", true},
		{"*", "anything.at.all", true},
		{"container[*].resources.*", "container[api].resources.limits.memory", true},
		{"data.*", "data.application.yml", true},
		{"data.*", "replicas", false},
		// A bare literal must not accidentally match a longer path.
		{"image", "container[api].image.tag", false},
	}
	for _, tc := range tests {
		t.Run(tc.pattern+" vs "+tc.path, func(t *testing.T) {
			if got := MatchPath(tc.pattern, tc.path); got != tc.want {
				t.Errorf("MatchPath(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
			}
		})
	}
}
