package render

import (
	"encoding/json"
	"io"
)

// JSON writes a TaskExport as indented JSON.
func JSON(w io.Writer, export *TaskExport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(export)
}
