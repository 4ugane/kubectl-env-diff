package report

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("writing golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden %s: %v (run with -update to create)", path, err)
	}
	if got != string(want) {
		t.Errorf("output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func sampleReport() Report {
	meta := Meta{FromContext: "staging", FromNamespace: "web",
		ToContext: "prod-au", ToNamespace: "web"}
	diffs := []model.Difference{
		{Kind: model.KindDeployment, Name: "notifier", Type: model.MissingInTo,
			Severity: model.SeverityDrift, From: "notifier"},
		{Kind: model.KindDeployment, Name: "checkout", Path: "container[checkout].env.FEATURE_FLAG_X",
			Type: model.MissingInFrom, Severity: model.SeverityDrift, To: "true"},
		{Kind: model.KindDeployment, Name: "checkout", Path: "container[checkout].env.DB_PASSWORD",
			Type: model.ValueChanged, Severity: model.SeverityDrift,
			From: "<redacted>", To: "<redacted>", Redacted: true},
		{Kind: model.KindDeployment, Name: "checkout", Path: "container[checkout].env.LOG_LEVEL",
			Type: model.ValueChanged, Severity: model.SeverityDrift, From: "debug", To: "info"},
		{Kind: model.KindDeployment, Name: "checkout", Path: "container[checkout].resources.limits.memory",
			Type: model.ValueChanged, Severity: model.SeverityDrift, From: "4Gi", To: "512Mi"},
		{Kind: model.KindDeployment, Name: "checkout", Path: "container[checkout].image.tag",
			Type: model.ValueChanged, Severity: model.SeverityExpected, From: "v1", To: "v2"},
		{Kind: model.KindConfigMap, Name: "app-config", Path: "data.KAFKA_BROKERS",
			Type: model.MissingInFrom, Severity: model.SeverityDrift, To: "b-1.msk.internal:9092"},
	}
	return Report{
		Summary: Summary{Meta: meta, Pairs: 12, Paired: 11, Identical: 9, Drifted: 2,
			MissingInTo: 1, ExpectedHidden: 1},
		Differences: diffs,
	}
}

func TestTextGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := Text(&buf, sampleReport(), TextOptions{Width: 100}); err != nil {
		t.Fatalf("Text() error = %v", err)
	}
	golden(t, "text_basic.golden", buf.String())
}

func TestTextRedactedShowsDiffersMarker(t *testing.T) {
	var buf bytes.Buffer
	if err := Text(&buf, sampleReport(), TextOptions{Width: 100}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "<redacted, differs>") {
		t.Error("redacted difference should be marked as differing")
	}
}

func TestTextExpectedCollapsedByDefault(t *testing.T) {
	var buf bytes.Buffer
	if err := Text(&buf, sampleReport(), TextOptions{Width: 100}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "image.tag") {
		t.Error("expected differences should be collapsed by default")
	}
	if !strings.Contains(out, "1 expected difference") {
		t.Errorf("expected count line missing:\n%s", out)
	}
}

func TestTextShowExpected(t *testing.T) {
	var buf bytes.Buffer
	if err := Text(&buf, sampleReport(), TextOptions{Width: 100, ShowExpected: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "image.tag") {
		t.Error("--show-expected should display expected differences")
	}
}

func TestTextTruncation(t *testing.T) {
	long := strings.Repeat("x", 300)
	r := Report{
		Summary: Summary{Meta: Meta{FromContext: "a", ToContext: "b"}, Paired: 1, Drifted: 1},
		Differences: []model.Difference{{
			Kind: model.KindDeployment, Name: "api", Path: "container[api].env.BLOB",
			Type: model.ValueChanged, Severity: model.SeverityDrift, From: long, To: "short",
		}},
	}

	var narrow bytes.Buffer
	if err := Text(&narrow, r, TextOptions{Width: 80}); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(narrow.String(), "\n") {
		if len([]rune(line)) > 80 {
			t.Fatalf("line exceeds width 80 (%d runes): %q", len([]rune(line)), line)
		}
	}
	if !strings.Contains(narrow.String(), "…") {
		t.Error("truncated value should be marked with an ellipsis")
	}

	var full bytes.Buffer
	if err := Text(&full, r, TextOptions{Width: 80, Full: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(full.String(), long) {
		t.Error("--full should print the untruncated value")
	}
}

func TestTextSkippedKindWarning(t *testing.T) {
	r := Report{Summary: Summary{
		Meta:    Meta{FromContext: "staging", ToContext: "prod"},
		Skipped: []model.SkipNote{{Kind: model.KindStatefulSet, Reason: "forbidden: cannot list in staging"}},
	}}
	var buf bytes.Buffer
	if err := Text(&buf, r, TextOptions{Width: 100}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "WARNING") {
		t.Error("skipped kinds must produce a loud warning")
	}
	if !strings.Contains(out, "StatefulSet") || !strings.Contains(out, "forbidden") {
		t.Errorf("warning should name the kind and reason:\n%s", out)
	}
}

func TestTextNoDrift(t *testing.T) {
	r := Report{Summary: Summary{
		Meta: Meta{FromContext: "staging", FromNamespace: "web",
			ToContext: "prod", ToNamespace: "web"},
		Pairs: 5, Paired: 5, Identical: 5,
	}}
	var buf bytes.Buffer
	if err := Text(&buf, r, TextOptions{Width: 100}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "no drift") {
		t.Errorf("clean run should say so plainly:\n%s", buf.String())
	}
}

func TestTextNoANSIWhenColorDisabled(t *testing.T) {
	var buf bytes.Buffer
	if err := Text(&buf, sampleReport(), TextOptions{Width: 100, Color: false}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\x1b[") {
		t.Error("ANSI escape emitted with colour disabled")
	}
}

// A long field PATH must be truncated the same way a long VALUE is: with an
// ellipsis marker, and never when --full is set. An earlier version sliced the
// path unconditionally with no marker at all, which silently dropped the end
// of a long path ("resources.limits.memory" read as "resources.limits") --
// the opposite of what a precise diff tool should do.
func TestTextLongPathTruncatedWithEllipsis(t *testing.T) {
	long := "container[api].resources.limits.some-extremely-long-field-name"
	r := Report{
		Summary: Summary{Meta: Meta{FromContext: "a", ToContext: "b"}, Paired: 1, Drifted: 1},
		Differences: []model.Difference{{
			Kind: model.KindDeployment, Name: "api", Path: long,
			Type: model.ValueChanged, Severity: model.SeverityDrift, From: "x", To: "y",
		}},
	}

	var truncated bytes.Buffer
	if err := Text(&truncated, r, TextOptions{Width: 100}); err != nil {
		t.Fatal(err)
	}
	out := truncated.String()
	if strings.Contains(out, long) {
		t.Error("long path should be truncated by default")
	}
	if !strings.Contains(out, "…") {
		t.Error("truncated path should be marked with an ellipsis, not silently cut")
	}

	var full bytes.Buffer
	if err := Text(&full, r, TextOptions{Width: 100, Full: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(full.String(), long) {
		t.Error("--full should print the untruncated path")
	}
}
