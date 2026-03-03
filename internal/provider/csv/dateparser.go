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
// that successfully parses at least 90% of them.
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

	for _, format := range dateFormats {
		successCount := 0
		for _, s := range cleaned {
			if _, err := time.Parse(format, s); err == nil {
				successCount++
			}
		}
		if successCount >= threshold {
			return format, nil
		}
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
