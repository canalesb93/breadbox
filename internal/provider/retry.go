package provider

import (
	"context"
	"time"
)

// RetryConfig controls exponential backoff behavior.
type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

// DefaultRetryConfig returns the standard retry configuration used by all
// providers: 5 retries, 2s initial delay, 60s maximum delay.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:   5,
		InitialDelay: 2 * time.Second,
		MaxDelay:     60 * time.Second,
	}
}

// DoWithRetry executes fn with exponential backoff. fn returns whether the call
// should be retried and any error. Retries stop when fn returns shouldRetry
// false, the maximum number of retries is exhausted, or the context is
// cancelled. When retries are exhausted the last error from fn is returned.
func DoWithRetry(ctx context.Context, cfg RetryConfig, fn func() (shouldRetry bool, err error)) error {
	delay := cfg.InitialDelay

	for attempt := 0; ; attempt++ {
		shouldRetry, err := fn()
		if !shouldRetry || err == nil {
			return err
		}

		if attempt >= cfg.MaxRetries {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		delay *= 2
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
	}
}
