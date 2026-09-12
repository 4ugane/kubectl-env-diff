package report

import (
	"encoding/json"
	"io"
)

// JSON renders the report as indented JSON. Field order is fixed by the type
// definitions and the difference slice is already sorted, so output is
// byte-identical across runs and can be diffed in CI.
func JSON(w io.Writer, r Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}
