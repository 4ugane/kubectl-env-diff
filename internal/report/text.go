package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

// TextOptions controls terminal rendering. Width and Color come from the caller
// rather than being detected here, keeping the renderer a pure function.
type TextOptions struct {
	Width        int
	Color        bool
	ShowExpected bool
	Full         bool
}

const (
	minWidth     = 40
	defaultWidth = 100
	pathColumn   = 34
	ansiReset    = "\x1b[0m"
	ansiBold     = "\x1b[1m"
	ansiRed      = "\x1b[31m"
	ansiYellow   = "\x1b[33m"
	ansiDim      = "\x1b[2m"
)

// Text renders the report for a terminal.
func Text(w io.Writer, r Report, opts TextOptions) error {
	width := opts.Width
	if width <= 0 {
		width = defaultWidth
	}
	if width < minWidth {
		width = minWidth
	}
	p := &printer{w: w, color: opts.Color, width: width}

	s := r.Summary
	p.line("kubectl env-diff   %s/%s -> %s/%s",
		s.FromContext, s.FromNamespace, s.ToContext, s.ToNamespace)
	p.blank()
	p.bold("SUMMARY   %d resources - %d identical - %d drifted - %d missing",
		s.Pairs, s.Identical, s.Drifted, s.MissingInFrom+s.MissingInTo)
	p.blank()

	for _, g := range r.GroupByResource() {
		p.colored(ansiRed, "DRIFT  %s/%s", g.Kind, g.Name)
		for _, d := range g.Differences {
			p.diffLine(d, opts.Full)
		}
		p.blank()
	}

	if missing := r.Missing(model.MissingInTo); len(missing) > 0 {
		p.colored(ansiYellow, "MISSING IN %s/%s", s.ToContext, s.ToNamespace)
		for _, d := range missing {
			p.line("  %s/%s", d.Kind, d.Name)
		}
		p.blank()
	}
	if missing := r.Missing(model.MissingInFrom); len(missing) > 0 {
		p.colored(ansiYellow, "MISSING IN %s/%s", s.FromContext, s.FromNamespace)
		for _, d := range missing {
			p.line("  %s/%s", d.Kind, d.Name)
		}
		p.blank()
	}

	if opts.ShowExpected {
		var expected []model.Difference
		for _, d := range r.Differences {
			if d.Severity == model.SeverityExpected {
				expected = append(expected, d)
			}
		}
		if len(expected) > 0 {
			p.colored(ansiDim, "EXPECTED")
			for _, d := range expected {
				p.diffLine(d, opts.Full)
			}
			p.blank()
		}
	} else if s.ExpectedHidden > 0 {
		p.colored(ansiDim, "%s hidden (image tag, replicas) - --show-expected to display",
			plural(s.ExpectedHidden, "expected difference", "expected differences"))
		p.blank()
	}

	// The warning goes last so it is the final thing on screen. A report that
	// silently skipped resources would be trusted, and wrong.
	if r.Incomplete() {
		p.colored(ansiRed, "WARNING: the comparison is incomplete")
		for _, sk := range s.Skipped {
			p.line("  skipped %s: %s", sk.Kind, sk.Reason)
		}
		p.line("  Treat the result above as partial. Exit code is 1, not 0 or 2.")
		p.blank()
	}

	if !r.HasDrift() && !r.Incomplete() {
		p.line("no drift found")
	}
	return p.err
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// printer accumulates the first write error rather than checking every call.
type printer struct {
	w     io.Writer
	color bool
	width int
	err   error
}

func (p *printer) line(format string, args ...any) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintln(p.w, fmt.Sprintf(format, args...))
}

func (p *printer) blank() { p.line("") }

func (p *printer) bold(format string, args ...any) { p.colored(ansiBold, format, args...) }

func (p *printer) colored(code, format string, args ...any) {
	text := fmt.Sprintf(format, args...)
	if p.color {
		text = code + text + ansiReset
	}
	p.line("%s", text)
}

// diffLine renders one difference, truncating values so the line fits.
//
// True side-by-side is deliberately not attempted: at 80 columns with long
// values it is unreadable, which is what the HTML renderer exists for.
func (p *printer) diffLine(d model.Difference, full bool) {
	from, to := displayValues(d)

	label := d.Path
	if !full {
		label = truncate(label, pathColumn)
	}

	budget := p.width - pathColumn - 9
	if budget < 12 {
		budget = 12
	}
	if !full {
		half := budget / 2
		from = truncate(from, half)
		to = truncate(to, half)
	}

	if d.Multiline {
		p.line("  %-*s", pathColumn, label)
		for _, l := range strings.Split(from, "\n") {
			p.line("    - %s", l)
		}
		for _, l := range strings.Split(to, "\n") {
			p.line("    + %s", l)
		}
		return
	}
	p.line("  %-*s  %s -> %s", pathColumn, label, from, to)
}

// truncate shortens a value, marking the cut with an ellipsis.
func truncate(s string, max int) string {
	if max < 4 {
		max = 4
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
