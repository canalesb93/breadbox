//go:build !lite

package agent

import "context"

// Semaphore is a bounded concurrency token implemented as a buffered channel.
// v1 default capacity is 1 — only one agent run at a time across the process.
type Semaphore struct {
	ch chan struct{}
}

// NewSemaphore creates a semaphore with the given capacity.
// Values < 1 are clamped to 1.
func NewSemaphore(capacity int) *Semaphore {
	if capacity < 1 {
		capacity = 1
	}
	return &Semaphore{ch: make(chan struct{}, capacity)}
}

// Acquire tries to grab a slot non-blockingly. Returns ErrConcurrencyLocked
// immediately when the semaphore is full. ctx is accepted for future
// blocking-acquire variants but currently unused.
func (s *Semaphore) Acquire(_ context.Context) error {
	select {
	case s.ch <- struct{}{}:
		return nil
	default:
		return ErrConcurrencyLocked
	}
}

// Release returns a slot to the semaphore. Always pair with a successful
// Acquire via defer.
func (s *Semaphore) Release() {
	select {
	case <-s.ch:
	default:
		// Already drained — pairing bug; ignore rather than panic.
	}
}

// Available returns how many slots are currently free. For tests only.
func (s *Semaphore) Available() int {
	return cap(s.ch) - len(s.ch)
}
