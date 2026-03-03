package csv

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
)

// ParsedFile holds the result of parsing a CSV/TSV file.
type ParsedFile struct {
	Headers   []string
	Rows      [][]string
	Delimiter rune
}

// ParseFile reads raw bytes, strips BOM, detects the delimiter, and parses
// headers + data rows. It rejects files with fewer than 2 rows or more than
// 50,000 rows.
func ParseFile(raw []byte) (*ParsedFile, error) {
	raw = stripBOM(raw)

	if len(raw) == 0 {
		return nil, errors.New("file is empty")
	}

	delim := detectDelimiter(raw)

	r := csv.NewReader(bytes.NewReader(raw))
	r.Comma = delim
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	var allRows [][]string
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse CSV: %w", err)
		}
		allRows = append(allRows, record)
	}

	if len(allRows) < 2 {
		return nil, errors.New("file must have at least 2 rows (header + 1 data row)")
	}
	if len(allRows) > 50001 { // header + 50,000 data rows
		return nil, fmt.Errorf("file has %d data rows, maximum is 50,000", len(allRows)-1)
	}

	return &ParsedFile{
		Headers:   allRows[0],
		Rows:      allRows[1:],
		Delimiter: delim,
	}, nil
}

// stripBOM removes UTF-8 and UTF-16 BOMs from the beginning of the file.
func stripBOM(raw []byte) []byte {
	// UTF-8 BOM
	if bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
		return raw[3:]
	}
	// UTF-16 LE BOM
	if bytes.HasPrefix(raw, []byte{0xFF, 0xFE}) {
		return raw[2:]
	}
	// UTF-16 BE BOM
	if bytes.HasPrefix(raw, []byte{0xFE, 0xFF}) {
		return raw[2:]
	}
	return raw
}

// detectDelimiter tries comma, tab, semicolon, and pipe on the first 5 rows
// to find the delimiter that produces consistent column counts.
func detectDelimiter(raw []byte) rune {
	candidates := []rune{',', '\t', ';', '|'}

	// Limit to first ~5 lines for detection.
	lines := bytes.SplitN(raw, []byte("\n"), 7)
	if len(lines) > 6 {
		lines = lines[:6]
	}
	sample := bytes.Join(lines, []byte("\n"))

	bestDelim := ','
	bestScore := 0

	for _, delim := range candidates {
		r := csv.NewReader(bytes.NewReader(sample))
		r.Comma = delim
		r.LazyQuotes = true
		r.TrimLeadingSpace = true

		var counts []int
		for {
			record, err := r.Read()
			if err != nil {
				break
			}
			counts = append(counts, len(record))
		}

		if len(counts) < 2 {
			continue
		}

		// Check consistency: all rows should have the same column count.
		first := counts[0]
		if first < 2 {
			continue
		}
		consistent := true
		for _, c := range counts[1:] {
			if c != first {
				consistent = false
				break
			}
		}

		score := 0
		if consistent {
			score = first * len(counts) // prefer more columns and more rows
		}

		if score > bestScore {
			bestScore = score
			bestDelim = delim
		}
	}

	return bestDelim
}
