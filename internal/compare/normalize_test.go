package compare

import (
	"strings"
	"testing"

	"github.com/4ugane/kubectl-env-diff/internal/config"
)

func TestNormalizeName(t *testing.T) {
	rules := config.NormalizeRules{
		StripSuffixes: []string{"-prod", "-staging", "-staging-canary"},
		StripPrefixes: []string{"svc-"},
	}
	tests := []struct{ in, want string }{
		{"api-staging", "api"},
		{"api-prod", "api"},
		{"api", "api"},
		// longest matching suffix wins, deterministically
		{"api-staging-canary", "api"},
		{"svc-api-prod", "api"},
		{"unrelated", "unrelated"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := NormalizeName(tc.in, rules)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if got != tc.want {
				t.Errorf("NormalizeName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A name that is entirely the suffix would normalize to nothing.
func TestNormalizeNameRejectsEmptyResult(t *testing.T) {
	rules := config.NormalizeRules{StripSuffixes: []string{"-prod", "prod"}}
	_, err := NormalizeName("prod", rules)
	if err == nil {
		t.Fatal("normalizing to an empty name should error")
	}
	if !strings.Contains(err.Error(), "prod") {
		t.Errorf("error should name the offender, got %v", err)
	}
}

func TestNormalizeNameNoRules(t *testing.T) {
	got, err := NormalizeName("api-staging", config.NormalizeRules{})
	if err != nil || got != "api-staging" {
		t.Errorf("got %q, %v; want unchanged", got, err)
	}
}
