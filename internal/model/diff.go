package model

import "strings"

// DiffType classifies what kind of change was found. Missing entries are the
// most valuable findings, so renderers show them first.
type DiffType string

const (
	ValueChanged  DiffType = "ValueChanged"
	MissingInTo   DiffType = "MissingInTo"
	MissingInFrom DiffType = "MissingInFrom"
)

// Severity is assigned by the classify package, never by compare.
type Severity string

const (
	SeverityDrift    Severity = "Drift"
	SeverityExpected Severity = "Expected"
)

// Difference is one reportable change. Path uses a human-readable form such as
// "container[api].resources.limits.memory".
//
// From and To hold display strings only. When a value was redacted at
// extraction, Redacted is true and the strings are placeholders — there is no
// code path by which a raw credential reaches this struct.
type Difference struct {
	Kind      Kind     `json:"kind"`
	Name      string   `json:"name"`
	FromName  string   `json:"fromName,omitempty"`
	ToName    string   `json:"toName,omitempty"`
	Path      string   `json:"path"`
	From      string   `json:"from"`
	To        string   `json:"to"`
	Type      DiffType `json:"type"`
	Severity  Severity `json:"severity"`
	Redacted  bool     `json:"redacted,omitempty"`
	Multiline bool     `json:"multiline,omitempty"`
}

// Category groups a difference for the HTML heatmap.
func (d Difference) Category() string {
	switch {
	case d.Path == "":
		return "workload"
	case strings.Contains(d.Path, ".env."), strings.HasPrefix(d.Path, "data."):
		return "env"
	case strings.Contains(d.Path, ".resources."):
		return "resources"
	case strings.Contains(d.Path, ".probes."):
		return "probes"
	case strings.Contains(d.Path, ".image"):
		return "image"
	case d.Path == "replicas":
		return "replicas"
	default:
		return "other"
	}
}
