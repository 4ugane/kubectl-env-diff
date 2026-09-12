package report

import (
	"bytes"
	"strings"
	"testing"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

func TestMarkdownGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := Markdown(&buf, sampleReport(), false); err != nil {
		t.Fatalf("Markdown() error = %v", err)
	}
	golden(t, "markdown_basic.golden", buf.String())
}

func TestMarkdownEscapesTableBreakingCharacters(t *testing.T) {
	// Context names deliberately avoid "a"/"b": Markdown()'s header row renders
	// them as plain column labels ("| ... | a | b |"), and a single-letter name
	// would coincidentally contain the same "a | b" substring the assertion
	// below checks for in the escaped VALUE — a false failure with nothing to
	// do with escaping. staging/prod cannot collide with the data under test.
	r := Report{
		Summary: Summary{Meta: Meta{FromContext: "staging", ToContext: "prod"}, Paired: 1, Drifted: 1},
		Differences: []model.Difference{{
			Kind: model.KindDeployment, Name: "api", Path: "container[api].env.CMD",
			Type: model.ValueChanged, Severity: model.SeverityDrift,
			From: "sh -c 'a | b'", To: "sh -c x",
		}},
	}
	var buf bytes.Buffer
	if err := Markdown(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if strings.Contains(out, "a | b") {
		t.Error("raw pipe survived into the table and will corrupt it")
	}
	if !strings.Contains(out, `a \| b`) {
		t.Errorf("pipe should be backslash-escaped:\n%s", out)
	}

	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "|") {
			continue
		}
		if n := countUnescapedPipes(line); n != 6 {
			t.Errorf("row has %d separators, want 6: %q", n, line)
		}
	}
}

func countUnescapedPipes(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '|' {
			continue
		}
		if i > 0 && s[i-1] == '\\' {
			continue
		}
		n++
	}
	return n
}

func TestMarkdownEscapeConvertsNewline(t *testing.T) {
	if got := mdEscape("a\nb"); got != "a<br>b" {
		t.Errorf("mdEscape = %q, want %q", got, "a<br>b")
	}
	if got := mdEscape("a\r\nb"); got != "a<br>b" {
		t.Errorf("CRLF should collapse to one <br>, got %q", got)
	}
}

func TestMarkdownEscapeBacktick(t *testing.T) {
	if got := mdEscape("a`b"); !strings.Contains(got, "\\`") {
		t.Errorf("backtick should be escaped, got %q", got)
	}
}

func TestMarkdownMultilineValueAppears(t *testing.T) {
	r := Report{
		Summary: Summary{Meta: Meta{FromContext: "a", ToContext: "b"}, Paired: 1, Drifted: 1},
		Differences: []model.Difference{{
			Kind: model.KindConfigMap, Name: "app-config", Path: "data.application.yml",
			Type: model.ValueChanged, Severity: model.SeverityDrift, Multiline: true,
			From: "L2: timeout: 30", To: "L2: timeout: 60",
		}},
	}
	var buf bytes.Buffer
	if err := Markdown(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "timeout: 30") {
		t.Error("changed line should appear in the table")
	}
}

func TestMarkdownSkippedWarning(t *testing.T) {
	r := Report{Summary: Summary{
		Meta:    Meta{FromContext: "staging", ToContext: "prod"},
		Skipped: []model.SkipNote{{Kind: model.KindStatefulSet, Reason: "forbidden"}},
	}}
	var buf bytes.Buffer
	if err := Markdown(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "incomplete") {
		t.Error("skipped kinds must be called out in markdown too")
	}
}

func TestMarkdownNoDrift(t *testing.T) {
	r := Report{Summary: Summary{Meta: Meta{FromContext: "a", ToContext: "b"}, Paired: 3, Identical: 3}}
	var buf bytes.Buffer
	if err := Markdown(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No drift") {
		t.Errorf("clean run should say so:\n%s", buf.String())
	}
}

// GFM tables allow raw inline HTML. An unescaped "<redacted>" is a
// well-formed but unrecognized tag with no content and no closing tag, and a
// renderer that passes raw HTML through (GitHub included) may render it as
// nothing rather than as visible text — silently erasing the one thing this
// tool exists to show ("it drifted, but not what to"). Angle brackets must
// become HTML entities so they always display literally.
func TestMarkdownEscapesAngleBrackets(t *testing.T) {
	r := Report{
		Summary: Summary{Meta: Meta{FromContext: "staging", ToContext: "prod"}, Paired: 1, Drifted: 1},
		Differences: []model.Difference{{
			Kind: model.KindDeployment, Name: "api", Path: "container[api].env.DB_PASSWORD",
			Type: model.ValueChanged, Severity: model.SeverityDrift, Redacted: true,
		}},
	}
	var buf bytes.Buffer
	if err := Markdown(&buf, r, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if strings.Contains(out, "<redacted") {
		t.Errorf("raw <redacted> tag reached markdown output, a renderer may swallow it:\n%s", out)
	}
	if !strings.Contains(out, "&lt;redacted&gt;") {
		t.Errorf("expected the escaped placeholder to appear literally, got:\n%s", out)
	}

	// The multi-line feature's own <br> insertion must survive, unescaped.
	multi := Report{
		Summary: Summary{Meta: Meta{FromContext: "a", ToContext: "b"}, Paired: 1, Drifted: 1},
		Differences: []model.Difference{{
			Kind: model.KindConfigMap, Name: "app-config", Path: "data.application.yml",
			Type: model.ValueChanged, Severity: model.SeverityDrift, Multiline: true,
			From: "L1: x", To: "L1: y\nL2: z",
		}},
	}
	var mbuf bytes.Buffer
	if err := Markdown(&mbuf, multi, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mbuf.String(), "<br>") {
		t.Error("intentional <br> for a multi-line value must not itself be escaped")
	}
}
