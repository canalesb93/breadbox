//go:build !lite

package service

import (
	"strings"

	lcron "github.com/lnquy/cron"
	rcron "github.com/robfig/cron/v3"
)

// cronDescriptor renders a cron expression as an English sentence. Built
// once at package init (stateless + concurrency-safe). English-only keeps
// the embedded locale table small.
var cronDescriptor = func() *lcron.ExpressionDescriptor {
	d, err := lcron.NewDescriptor(
		lcron.SetLocales(lcron.Locale_en),
		lcron.Use24HourTimeFormat(false),
	)
	if err != nil {
		return nil
	}
	return d
}()

// DescribeCron validates a standard 5-field cron expression and returns a
// human-readable description of when it fires (e.g. "At 07:00 PM, only on
// Tuesday and Thursday"). Returns valid=false with a short reason when the
// expression doesn't parse. Powers the workflow configure drawer's live
// schedule preview.
//
// Validation uses the same parser the scheduler registers with
// (robfig/cron ParseStandard) so the preview never disagrees with what
// actually runs; the English rendering comes from lnquy/cron.
func (s *Service) DescribeCron(expr string) (valid bool, description string) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false, "Enter a schedule"
	}
	if _, err := rcron.ParseStandard(expr); err != nil {
		return false, "Not a valid cron expression"
	}
	if cronDescriptor == nil {
		return true, "Custom schedule"
	}
	desc, err := cronDescriptor.ToDescription(expr, lcron.Locale_en)
	if err != nil || strings.TrimSpace(desc) == "" {
		// Parsed fine but couldn't render — still valid, just generic.
		return true, "Custom schedule"
	}
	return true, desc
}
