package provider

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDoWithRetry_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	err := DoWithRetry(context.Background(), DefaultRetryConfig(), func() (bool, error) {
		calls++
		return false, nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDoWithRetry_SuccessAfterRetries(t *testing.T) {
	cfg := RetryConfig{MaxRetries: 5, InitialDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}
	calls := 0
	err := DoWithRetry(context.Background(), cfg, func() (bool, error) {
		calls++
		if calls < 3 {
			return true, errors.New("transient")
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDoWithRetry_MaxRetriesExhausted(t *testing.T) {
	cfg := RetryConfig{MaxRetries: 3, InitialDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}
	calls := 0
	sentinel := errors.New("always fail")
	err := DoWithRetry(context.Background(), cfg, func() (bool, error) {
		calls++
		return true, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	// MaxRetries=3 means attempts 0,1,2,3 then exhausted on attempt 3.
	// That is 4 total calls.
	if calls != 4 {
		t.Fatalf("expected 4 calls (initial + 3 retries), got %d", calls)
	}
}

func TestDoWithRetry_ContextCancellation(t *testing.T) {
	cfg := RetryConfig{MaxRetries: 10, InitialDelay: time.Second, MaxDelay: 10 * time.Second}
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	err := DoWithRetry(ctx, cfg, func() (bool, error) {
		calls++
		// Cancel context after first call so the retry wait returns immediately.
		cancel()
		return true, errors.New("transient")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call before cancellation, got %d", calls)
	}
}

func TestDoWithRetry_BackoffTiming(t *testing.T) {
	cfg := RetryConfig{MaxRetries: 4, InitialDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond}

	var delays []time.Duration
	last := time.Now()

	calls := 0
	_ = DoWithRetry(context.Background(), cfg, func() (bool, error) {
		now := time.Now()
		if calls > 0 {
			delays = append(delays, now.Sub(last))
		}
		last = now
		calls++
		return true, errors.New("fail")
	})

	// Expected delays: 10ms, 20ms, 40ms, 50ms (capped)
	expected := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}

	if len(delays) != len(expected) {
		t.Fatalf("expected %d delays, got %d", len(expected), len(delays))
	}

	for i, exp := range expected {
		// Allow 50% tolerance for timer imprecision.
		low := exp / 2
		high := exp * 3
		if delays[i] < low || delays[i] > high {
			t.Errorf("delay[%d] = %v, expected ~%v (range %v–%v)", i, delays[i], exp, low, high)
		}
	}
}

func TestDoWithRetry_NoRetryOnNilError(t *testing.T) {
	// Even if shouldRetry is true, nil error means success.
	calls := 0
	err := DoWithRetry(context.Background(), DefaultRetryConfig(), func() (bool, error) {
		calls++
		return true, nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDoWithRetry_ZeroMaxRetries(t *testing.T) {
	cfg := RetryConfig{MaxRetries: 0, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond}
	calls := 0
	sentinel := errors.New("fail")
	err := DoWithRetry(context.Background(), cfg, func() (bool, error) {
		calls++
		return true, sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call with zero retries, got %d", calls)
	}
}
