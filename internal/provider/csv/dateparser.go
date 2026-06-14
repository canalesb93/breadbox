//go:build !lite

package csv

import (
	"fmt"
	"strings"
	"time"
)

// dateFormats lists supported date formats in priority order.
// Go reference time: Mon Jan 2 15:04:05 MST 2006
var dateFormats = []string{
	"01/02/2006",       // MM/DD/YYYY
	"2006-01-02",       // YYYY-MM-DD (ISO 8601)
	"1/2/2006",         // M/D/YYYY
	"01-02-2006",       // MM-DD-YYYY
	"02/01/2006",       // DD/MM/YYYY
	"2006/01/02",       // YYYY/MM/DD
	"Jan 2, 2006",      // Mon DD, YYYY
	"January 2, 2006",  // Month DD, YYYY
}

// DetectDateFormat samples the first 20 non-empty values and picks the format
// that parses the most of them, requiring at least 90% coverage. Among formats
// that meet the threshold, the one with the highest coverage wins; ties are
// broken by dateFormats priority order (so US MM/DD stays the default for
// ambiguous data). Preferring the highest-coverage format matters when a stricter
// format crosses the threshold first but a more permissive sibling parses every
// row — e.g. a file of mostly zero-padded "01/15/2024" dates with a few unpadded
// "1/5/2024" rows: "01/02/2006" parses only the padded majority while "1/2/2006"
// parses all of them. Picking the broader format avoids silently skipping the
// non-conforming rows at import time.
func DetectDateFormat(samples []string) (string, error) {
	// Collect up to 20 non-empty samples.
	var cleaned []string
	for _, s := range samples {
		s = strings.TrimSpace(s)
		if s != "" && len(cleaned) < 20 {
			cleaned = append(cleaned, s)
		}
	}

	if len(cleaned) == 0 {
		return "", fmt.Errorf("no date values to analyze")
	}

	threshold := int(float64(len(cleaned)) * 0.9)
	if threshold < 1 {
		threshold = 1
	}

	bestFormat := ""
	bestScore := 0
	for _, format := range dateFormats {
		successCount := 0
		for _, s := range cleaned {
			if _, err := time.Parse(format, s); err == nil {
				successCount++
			}
		}
		// Strict > keeps the earliest format on ties, preserving priority order.
		if successCount >= threshold && successCount > bestScore {
			bestFormat = format
			bestScore = successCount
		}
	}
	if bestFormat != "" {
		return bestFormat, nil
	}

	return "", fmt.Errorf("could not detect date format — none of the supported formats parsed 90%% of sample values")
}

// ParseDate parses a date string using the given Go time format.
func ParseDate(raw, format string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	t, err := time.Parse(format, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse date %q with format %q: %w", raw, format, err)
	}
	return t, nil
}
