package report

import (
	"embed"
	"html/template"
	"io"
	"sort"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

//go:embed template.html
var templateFS embed.FS

// heatmapMinRows is the number of drifted resources below which the heatmap is
// suppressed. A one- or two-row grid conveys nothing the detail table does not.
const heatmapMinRows = 3

// HeatCategories is the fixed column order. Fixed rather than derived so the
// grid looks the same across runs and across reports.
var HeatCategories = []string{"env", "resources", "probes", "image", "replicas", "other"}

// HeatRow is one resource's drift counts by category.
type HeatRow struct {
	Kind   model.Kind
	Name   string
	Counts map[string]int
	Total  int
}

// HeatmapData is the resource-by-category grid.
type HeatmapData struct {
	Categories     []string
	Rows           []HeatRow
	CategoryTotals map[string]int
	Max            int
}

// Heatmap builds the grid, or returns nil when there are too few resources for
// it to be meaningful.
//
// Its purpose is the systemic finding: "resource limits drift in 8 of 12
// services" is one problem, not eight, and no line-by-line format shows that.
func Heatmap(r Report) *HeatmapData {
	rows := map[string]*HeatRow{}
	totals := map[string]int{}

	for _, d := range r.Differences {
		if d.Severity != model.SeverityDrift || d.Path == "" {
			continue
		}
		key := string(d.Kind) + "/" + d.Name
		row, ok := rows[key]
		if !ok {
			row = &HeatRow{Kind: d.Kind, Name: d.Name, Counts: map[string]int{}}
			rows[key] = row
		}
		cat := d.Category()
		row.Counts[cat]++
		row.Total++
		totals[cat]++
	}

	if len(rows) < heatmapMinRows {
		return nil
	}

	h := &HeatmapData{Categories: HeatCategories, CategoryTotals: totals}
	for _, row := range rows {
		for _, c := range HeatCategories {
			if row.Counts[c] > h.Max {
				h.Max = row.Counts[c]
			}
		}
		h.Rows = append(h.Rows, *row)
	}
	sort.Slice(h.Rows, func(i, j int) bool {
		if h.Rows[i].Kind != h.Rows[j].Kind {
			return h.Rows[i].Kind < h.Rows[j].Kind
		}
		return h.Rows[i].Name < h.Rows[j].Name
	})
	return h
}

// htmlRow is one rendered detail row.
type htmlRow struct {
	Kind     model.Kind
	Name     string
	Path     string
	From     string
	To       string
	Type     model.DiffType
	Severity model.Severity
	Redacted bool
}

// htmlData is the template's input.
type htmlData struct {
	Summary       Summary
	Rows          []htmlRow
	MissingInTo   []model.Difference
	MissingInFrom []model.Difference
	Heatmap       *HeatmapData
	ShowExpected  bool
	Incomplete    bool
}

// HTML renders a single self-contained document.
//
// html/template auto-escapes every value, which matters here: env values and
// ConfigMap contents are untrusted input and can contain markup.
func HTML(w io.Writer, r Report, showExpected bool) error {
	tmpl, err := template.New("template.html").Funcs(template.FuncMap{
		"shade": shade,
		"count": func(m map[string]int, k string) int { return m[k] },
		"add":   func(a, b int) int { return a + b },
	}).ParseFS(templateFS, "template.html")
	if err != nil {
		return err
	}

	source := r.DriftOnly()
	if showExpected {
		source = r.Differences
	}

	data := htmlData{
		Summary:       r.Summary,
		MissingInTo:   r.Missing(model.MissingInTo),
		MissingInFrom: r.Missing(model.MissingInFrom),
		Heatmap:       Heatmap(r),
		ShowExpected:  showExpected,
		Incomplete:    r.Incomplete(),
	}
	for _, d := range source {
		if d.Path == "" {
			continue
		}
		from, to := displayValues(d)
		data.Rows = append(data.Rows, htmlRow{
			Kind: d.Kind, Name: d.Name, Path: d.Path,
			From: from, To: to, Type: d.Type,
			Severity: d.Severity, Redacted: d.Redacted,
		})
	}
	return tmpl.Execute(w, data)
}

// shade maps a count to one of five intensity buckets for the heatmap cells.
func shade(n, max int) string {
	if n == 0 || max == 0 {
		return "s0"
	}
	switch bucket := (n * 4) / max; bucket {
	case 0:
		return "s1"
	case 1:
		return "s2"
	case 2:
		return "s3"
	default:
		return "s4"
	}
}
