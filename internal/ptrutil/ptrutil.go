// Package ptrutil provides tiny generic helpers for working with pointer
// values. They exist so the same one-line pointer-deref boilerplate isn't
// re-implemented under a different name in every package.
package ptrutil

// Deref returns *p if non-nil, else the zero value of T.
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// DerefOr returns *p if non-nil, else def.
func DerefOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}
