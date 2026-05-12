package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestResolve_FlagsWin(t *testing.T) {
	var buf bytes.Buffer
	if got := Resolve(true, false, &buf); got != ModeJSON {
		t.Fatalf("Resolve(json=true) = %v want JSON", got)
	}
	if got := Resolve(false, true, &buf); got != ModeNDJSON {
		t.Fatalf("Resolve(ndjson=true) = %v want NDJSON", got)
	}
}

func TestResolve_BufferDefaultsToJSON(t *testing.T) {
	var buf bytes.Buffer
	if got := Resolve(false, false, &buf); got != ModeJSON {
		t.Fatalf("non-tty default = %v want JSON", got)
	}
}

func TestPrintJSON_Indented(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintJSON(&buf, map[string]string{"k": "v"}); err != nil {
		t.Fatalf("PrintJSON: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "  \"k\": \"v\"") {
		t.Fatalf("expected indented JSON, got: %s", out)
	}
}

func TestPrintNDJSON_OneObjectPerLine(t *testing.T) {
	var buf bytes.Buffer
	items := []any{
		map[string]int{"n": 1},
		map[string]int{"n": 2},
	}
	if err := PrintNDJSON(&buf, items); err != nil {
		t.Fatalf("PrintNDJSON: %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), buf.String())
	}
	for _, l := range lines {
		var v map[string]int
		if err := json.Unmarshal([]byte(l), &v); err != nil {
			t.Fatalf("line not JSON: %q (%v)", l, err)
		}
	}
}

func TestTable_HeaderAndRows(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, []string{"NAME", "STATUS"})
	tbl.AddRow("local", "default")
	tbl.AddRow("prod", "")
	if err := tbl.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "STATUS") {
		t.Fatalf("missing header: %q", out)
	}
	if !strings.Contains(out, "local") || !strings.Contains(out, "prod") {
		t.Fatalf("missing row: %q", out)
	}
}
