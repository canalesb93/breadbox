// Package output centralises the CLI's stdout formatting. Commands hand
// a value to one of the helpers here and the package picks the right
// shape (table for TTYs, JSON otherwise) based on user flags and stream
// detection. Keeping this in one place means every subcommand renders
// consistently and the rules for JSON/NDJSON detection live in exactly
// one file.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

// Mode selects the wire shape of a rendered response.
type Mode int

const (
	// ModeAuto picks Table when stdout is a TTY and JSON otherwise.
	ModeAuto Mode = iota
	ModeTable
	ModeJSON
	ModeNDJSON
)

// Resolve picks a concrete Mode given the user-provided flags. The
// agent-friendly default is JSON-when-piped-to-a-file; --json and
// --ndjson force the corresponding mode regardless of the TTY.
func Resolve(jsonFlag, ndjsonFlag bool, out io.Writer) Mode {
	if ndjsonFlag {
		return ModeNDJSON
	}
	if jsonFlag {
		return ModeJSON
	}
	if isTerminal(out) {
		return ModeTable
	}
	return ModeJSON
}

// isTerminal returns true if w is os.Stdout/Stderr attached to a TTY.
// Anything else (file, pipe, *bytes.Buffer in tests) is treated as
// machine-readable.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// PrintJSON encodes v as indented JSON to w.
func PrintJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// PrintNDJSON encodes each element of items on its own line. items must
// be a slice — pass `[]map[string]any{single}` for a one-item stream.
func PrintNDJSON(w io.Writer, items []any) error {
	enc := json.NewEncoder(w)
	for _, item := range items {
		if err := enc.Encode(item); err != nil {
			return err
		}
	}
	return nil
}

// Table is a column-aware writer suitable for human terminal output.
// Header rows are mandatory; every row passed to AddRow MUST have the
// same number of cells as the header.
type Table struct {
	w       *tabwriter.Writer
	header  []string
	written bool
}

// NewTable builds a Table backed by tabwriter.
func NewTable(w io.Writer, header []string) *Table {
	t := &Table{
		w:      tabwriter.NewWriter(w, 0, 0, 2, ' ', 0),
		header: header,
	}
	return t
}

// AddRow appends a row. Cell count must match the header length.
func (t *Table) AddRow(cells ...string) {
	if !t.written {
		fmt.Fprintln(t.w, joinCells(t.header))
		t.written = true
	}
	fmt.Fprintln(t.w, joinCells(cells))
}

// Flush writes the table out. A Table with no rows still emits the
// header — callers wanting "(none)" placeholders should check the row
// count themselves before constructing the table.
func (t *Table) Flush() error {
	if !t.written {
		fmt.Fprintln(t.w, joinCells(t.header))
	}
	return t.w.Flush()
}

func joinCells(cells []string) string {
	if len(cells) == 0 {
		return ""
	}
	out := cells[0]
	for _, c := range cells[1:] {
		out += "\t" + c
	}
	return out
}
