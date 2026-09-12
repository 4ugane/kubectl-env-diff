package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

// Markdown renders a table that pastes directly into Slack, Jira, GitHub, and
// pull request comments.
func Markdown(w io.Writer, r Report, showExpected bool) error {
	var b strings.Builder
	s := r.Summary

	fmt.Fprintf(&b, "## env-diff: %s/%s to %s/%s\n\n",
		s.FromContext, s.FromNamespace, s.ToContext, s.ToNamespace)
	fmt.Fprintf(&b, "**%d resources** - %d identical - **%d drifted** - %d missing\n\n",
		s.Pairs, s.Identical, s.Drifted, s.MissingInFrom+s.MissingInTo)

	if r.Incomplete() {
		b.WriteString("> **This comparison is incomplete.** Some resources could not be read:\n>\n")
		for _, sk := range s.Skipped {
			fmt.Fprintf(&b, "> - %s: %s\n", sk.Kind, mdEscape(sk.Reason))
		}
		b.WriteString(">\n> Treat the results below as partial.\n\n")
	}

	rows := r.DriftOnly()
	if showExpected {
		rows = r.Differences
	}
	var tableRows []model.Difference
	for _, d := range rows {
		if d.Path != "" {
			tableRows = append(tableRows, d)
		}
	}

	if len(tableRows) > 0 {
		fmt.Fprintf(&b, "| Kind | Resource | Field | %s | %s |\n",
			mdEscape(s.FromContext), mdEscape(s.ToContext))
		b.WriteString("|---|---|---|---|---|\n")
		for _, d := range tableRows {
			from, to := displayValues(d)
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
				d.Kind, mdEscape(d.Name), mdEscape(d.Path), mdEscape(from), mdEscape(to))
		}
		b.WriteString("\n")
	}

	for _, dir := range []struct {
		typ   model.DiffType
		label string
	}{
		{model.MissingInTo, fmt.Sprintf("Missing in %s/%s", s.ToContext, s.ToNamespace)},
		{model.MissingInFrom, fmt.Sprintf("Missing in %s/%s", s.FromContext, s.FromNamespace)},
	} {
		missing := r.Missing(dir.typ)
		if len(missing) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n", dir.label)
		for _, d := range missing {
			fmt.Fprintf(&b, "- %s/%s\n", d.Kind, mdEscape(d.Name))
		}
		b.WriteString("\n")
	}

	if !showExpected && s.ExpectedHidden > 0 {
		fmt.Fprintf(&b, "_%s hidden (image tag, replicas)._\n\n",
			plural(s.ExpectedHidden, "expected difference", "expected differences"))
	}

	if !r.HasDrift() && !r.Incomplete() {
		b.WriteString("No drift found.\n")
	}

	_, err := io.WriteString(w, b.String())
	return err
}

// mdEscape makes a value safe inside a table cell.
//
// A raw pipe ends the cell and corrupts every following column; a raw backtick
// opens a code span that swallows the rest of the row; a newline ends the row
// entirely, so it becomes a <br>.
func mdEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "`", "\\`")
	// GFM table cells allow raw inline HTML. An unescaped "<redacted>" is a
	// well-formed (if unrecognized) HTML tag, and a renderer that passes raw
	// HTML through - GitHub's included - may swallow it rather than display
	// it literally, since it has no matching close tag and no content. That
	// would make the exact signal this tool exists to show ("it drifted, but
	// not what to") silently vanish instead of rendering. Escaping to HTML
	// entities keeps it visible as literal text in every renderer.
	//
	// This must run BEFORE the newline-to-<br> step below, so the <br> this
	// function deliberately inserts for multi-line values is not itself
	// escaped into &lt;br&gt;.
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", "<br>")
	return s
}
