package cli

import (
	"strings"
	"testing"
)

func TestValidateRejectsMutuallyExclusiveNames(t *testing.T) {
	o := &Options{From: "a/n", To: "b/n", Name: "api", FromName: "api-staging", ToName: "api-prod"}
	err := o.Validate()
	if err == nil {
		t.Fatal("--name with --from-name/--to-name should be rejected")
	}
	if !strings.Contains(err.Error(), "--name") {
		t.Errorf("error should name the offending flags, got %v", err)
	}
}

func TestValidateRequiresBothExplicitNames(t *testing.T) {
	for _, o := range []*Options{
		{From: "a/n", To: "b/n", FromName: "api-staging"},
		{From: "a/n", To: "b/n", ToName: "api-prod"},
	} {
		if err := o.Validate(); err == nil {
			t.Errorf("one explicit name without the other should be rejected: %+v", o)
		}
	}
}

func TestValidateRequiresFromAndTo(t *testing.T) {
	if err := (&Options{To: "b/n"}).Validate(); err == nil {
		t.Error("missing --from should be rejected")
	}
	if err := (&Options{From: "a/n"}).Validate(); err == nil {
		t.Error("missing --to should be rejected")
	}
}

// Same context AND namespace compares a thing to itself.
func TestValidateRejectsIdenticalTargets(t *testing.T) {
	err := (&Options{From: "staging/web", To: "staging/web"}).Validate()
	if err == nil {
		t.Fatal("identical context and namespace should be rejected")
	}
	if !strings.Contains(err.Error(), "itself") {
		t.Errorf("error should explain why, got %v", err)
	}
}

// Same context with DIFFERENT namespaces is a legitimate comparison.
func TestValidateAllowsSameContextDifferentNamespace(t *testing.T) {
	if err := (&Options{From: "staging/ns-a", To: "staging/ns-b"}).Validate(); err != nil {
		t.Errorf("same context, different namespaces should be allowed, got %v", err)
	}
}

func TestValidateOutputFormat(t *testing.T) {
	for _, f := range []string{"text", "json", "markdown", "html"} {
		if err := (&Options{From: "a/n", To: "b/n", Output: f}).Validate(); err != nil {
			t.Errorf("%q should be accepted, got %v", f, err)
		}
	}
	err := (&Options{From: "a/n", To: "b/n", Output: "yaml"}).Validate()
	if err == nil {
		t.Fatal("unknown output format should be rejected")
	}
	if !strings.Contains(err.Error(), "markdown") {
		t.Errorf("error should list valid formats, got %v", err)
	}
}

func TestValidateKinds(t *testing.T) {
	o := &Options{From: "a/n", To: "b/n", Kinds: []string{"deployment", "configmap"}}
	if err := o.Validate(); err != nil {
		t.Fatalf("valid kinds rejected: %v", err)
	}
	kinds, err := o.ResolvedKinds()
	if err != nil {
		t.Fatal(err)
	}
	if len(kinds) != 2 {
		t.Errorf("got %d kinds, want 2", len(kinds))
	}

	if err := (&Options{From: "a/n", To: "b/n", Kinds: []string{"pod"}}).Validate(); err == nil {
		t.Error("unknown kind should be rejected")
	}
}

func TestResolvedKindsDefaultsToAll(t *testing.T) {
	kinds, err := (&Options{From: "a/n", To: "b/n"}).ResolvedKinds()
	if err != nil {
		t.Fatal(err)
	}
	if len(kinds) != 3 {
		t.Errorf("got %d kinds, want 3 by default", len(kinds))
	}
}

func TestResolvedKindsDeduplicates(t *testing.T) {
	kinds, err := (&Options{Kinds: []string{"deploy", "deployment", "deployments"}}).ResolvedKinds()
	if err != nil {
		t.Fatal(err)
	}
	if len(kinds) != 1 {
		t.Errorf("aliases of one kind should collapse, got %v", kinds)
	}
}

func TestExitCodePrecedence(t *testing.T) {
	tests := []struct {
		name       string
		drift      bool
		incomplete bool
		want       int
	}{
		{"clean", false, false, 0},
		{"drift only", true, false, 2},
		// Incompleteness outranks drift: the run could not answer the question.
		{"incomplete only", false, true, 1},
		{"drift and incomplete", true, true, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCode(tc.drift, tc.incomplete); got != tc.want {
				t.Errorf("ExitCode(%v, %v) = %d, want %d", tc.drift, tc.incomplete, got, tc.want)
			}
		})
	}
}
