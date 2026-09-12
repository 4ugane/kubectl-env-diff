package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONShape(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, sampleReport()); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}

	var parsed struct {
		Summary struct {
			FromContext string `json:"fromContext"`
			Drifted     int    `json:"drifted"`
		} `json:"summary"`
		Differences []struct {
			Kind     string `json:"kind"`
			Path     string `json:"path"`
			Severity string `json:"severity"`
			Redacted bool   `json:"redacted"`
		} `json:"differences"`
	}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed.Summary.FromContext != "staging" {
		t.Errorf("fromContext = %q", parsed.Summary.FromContext)
	}
	if parsed.Summary.Drifted != 2 {
		t.Errorf("drifted = %d, want 2", parsed.Summary.Drifted)
	}
	if len(parsed.Differences) != 7 {
		t.Errorf("got %d differences, want 7", len(parsed.Differences))
	}
}

func TestJSONIncludesExpectedWithSeverity(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, sampleReport()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"severity": "Expected"`) {
		t.Error("expected differences should appear with their severity")
	}
}

func TestJSONCarriesOnlyRedactedPlaceholders(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, sampleReport()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "<redacted>") {
		t.Error("redacted placeholder missing")
	}
}

func TestJSONIsDeterministic(t *testing.T) {
	var first bytes.Buffer
	if err := JSON(&first, sampleReport()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		var next bytes.Buffer
		if err := JSON(&next, sampleReport()); err != nil {
			t.Fatal(err)
		}
		if first.String() != next.String() {
			t.Fatal("JSON output is not byte-identical across runs")
		}
	}
}
