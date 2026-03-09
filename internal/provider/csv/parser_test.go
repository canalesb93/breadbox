package csv

import (
	"strings"
	"testing"
)

func TestParseFileCSV(t *testing.T) {
	input := "Date,Amount,Description\n2024-01-15,42.50,Coffee Shop\n2024-01-16,10.00,Grocery Store\n"

	pf, err := ParseFile([]byte(input))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if len(pf.Headers) != 3 {
		t.Errorf("headers: got %d, want 3", len(pf.Headers))
	}
	if len(pf.Rows) != 2 {
		t.Errorf("rows: got %d, want 2", len(pf.Rows))
	}
	if pf.Delimiter != ',' {
		t.Errorf("delimiter: got %q, want ','", pf.Delimiter)
	}
}

func TestParseFileTSV(t *testing.T) {
	input := "Date\tAmount\tDescription\n2024-01-15\t42.50\tCoffee\n2024-01-16\t10.00\tGrocery\n"

	pf, err := ParseFile([]byte(input))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if pf.Delimiter != '\t' {
		t.Errorf("delimiter: got %q, want tab", pf.Delimiter)
	}
	if len(pf.Headers) != 3 {
		t.Errorf("headers: got %d, want 3", len(pf.Headers))
	}
}

func TestParseFileSemicolon(t *testing.T) {
	input := "Date;Amount;Description\n2024-01-15;42.50;Coffee\n2024-01-16;10.00;Grocery\n"

	pf, err := ParseFile([]byte(input))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if pf.Delimiter != ';' {
		t.Errorf("delimiter: got %q, want ';'", pf.Delimiter)
	}
}

func TestParseFilePipe(t *testing.T) {
	input := "Date|Amount|Description\n2024-01-15|42.50|Coffee\n2024-01-16|10.00|Grocery\n"

	pf, err := ParseFile([]byte(input))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if pf.Delimiter != '|' {
		t.Errorf("delimiter: got %q, want '|'", pf.Delimiter)
	}
}

func TestParseFileStripUTF8BOM(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	input := append(bom, []byte("A,B\n1,2\n")...)

	pf, err := ParseFile(input)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if pf.Headers[0] != "A" {
		t.Errorf("BOM not stripped: first header = %q", pf.Headers[0])
	}
}

func TestParseFileStripUTF16LEBOM(t *testing.T) {
	raw := []byte{0xFF, 0xFE}
	raw = append(raw, []byte("A,B\n1,2\n")...)

	pf, err := ParseFile(raw)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if len(pf.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(pf.Rows))
	}
}

func TestParseFileStripUTF16BEBOM(t *testing.T) {
	raw := []byte{0xFE, 0xFF}
	raw = append(raw, []byte("A,B\n1,2\n")...)

	pf, err := ParseFile(raw)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if len(pf.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(pf.Rows))
	}
}

func TestParseFileEmpty(t *testing.T) {
	_, err := ParseFile([]byte{})
	if err == nil {
		t.Error("expected error for empty file")
	}
}

func TestParseFileTooFewRows(t *testing.T) {
	_, err := ParseFile([]byte("Header1,Header2\n"))
	if err == nil {
		t.Error("expected error for file with only header row")
	}
}

func TestParseFileTooManyRows(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("A,B\n")
	for i := 0; i < 50001; i++ {
		sb.WriteString("1,2\n")
	}

	_, err := ParseFile([]byte(sb.String()))
	if err == nil {
		t.Error("expected error for file exceeding 50,000 rows")
	}
}

func TestStripBOMNoBOM(t *testing.T) {
	input := []byte("plain text")
	got := stripBOM(input)
	if string(got) != "plain text" {
		t.Errorf("stripBOM modified non-BOM input: %q", got)
	}
}
