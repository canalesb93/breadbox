package admin

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

// readAllCSV reads every record from `raw` and returns the first non-EOF
// error. Used to obtain a real *csv.ParseError for the tests below.
func readAllCSV(raw string) error {
	r := csv.NewReader(bytes.NewReader([]byte(raw)))
	for {
		_, err := r.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func TestHumanizeCSVError(t *testing.T) {
	t.Parallel()

	// Real encoding/csv field-count error: header has 3 columns, row 2 has 2.
	fieldCountErr := readAllCSV("a,b,c\nx,y\n")
	if fieldCountErr == nil {
		t.Fatal("expected ErrFieldCount from mismatched rows")
	}
	// Wrap the way internal/provider/csv.ParseFile does.
	wrappedFieldCount := fmt.Errorf("parse CSV: %w", fieldCountErr)

	// Real bare-quote error: quote appearing mid-field without LazyQuotes.
	bareQuoteErr := readAllCSV("a,b\nfoo\",bar\n")
	if bareQuoteErr == nil {
		t.Fatal("expected a bare-quote error")
	}
	wrappedBareQuote := fmt.Errorf("parse CSV: %w", bareQuoteErr)

	// Real extraneous/unclosed quote: opening " with no closing match.
	quoteErr := readAllCSV("a,b\n\"oops,bar\n")
	if quoteErr == nil {
		t.Fatal("expected a quote error")
	}
	wrappedQuote := fmt.Errorf("parse CSV: %w", quoteErr)

	tests := []struct {
		name         string
		err          error
		wantContains []string
		wantMissing  []string
	}{
		{
			name: "nil returns empty",
			err:  nil,
		},
		{
			name:         "field count surfaces row and plain language",
			err:          wrappedFieldCount,
			wantContains: []string{"Row 2", "different number of columns", "re-export"},
			wantMissing:  []string{"parse CSV", "wrong number of fields", "field"},
		},
		{
			name:         "bare quote",
			err:          wrappedBareQuote,
			wantContains: []string{"unescaped quote"},
			wantMissing:  []string{"parse CSV", "bare \""},
		},
		{
			name:         "unclosed quote",
			err:          wrappedQuote,
			wantContains: []string{"never closed"},
			wantMissing:  []string{"parse CSV"},
		},
		{
			name:         "unknown error strips parse CSV prefix",
			err:          fmt.Errorf("parse CSV: %w", errors.New("file is empty")),
			wantContains: []string{"file is empty"},
			wantMissing:  []string{"parse CSV:"},
		},
		{
			name:         "sentinel fallback still readable",
			err:          errors.New("file must have at least 2 rows (header + 1 data row)"),
			wantContains: []string{"at least 2 rows"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := humanizeCSVError(tt.err)
			if tt.err == nil {
				if got != "" {
					t.Fatalf("want empty string, got %q", got)
				}
				return
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("result %q missing expected substring %q", got, want)
				}
			}
			for _, dontWant := range tt.wantMissing {
				if strings.Contains(got, dontWant) {
					t.Errorf("result %q contains disallowed substring %q", got, dontWant)
				}
			}
		})
	}
}
