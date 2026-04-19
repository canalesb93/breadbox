// Package sliceutil provides small, dependency-free helpers for operating on
// []string. It exists so sync and service layers share one implementation of
// the case-insensitive tag/slug operations used during rule evaluation.
package sliceutil

import "strings"

// Contains reports whether slice contains target (case-sensitive).
func Contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// ContainsFold reports whether slice contains target (case-insensitive).
// An empty target never matches — callers comparing tag slugs should treat
// "no tag" as absent rather than matching empty strings in the slice.
func ContainsFold(slice []string, target string) bool {
	if target == "" {
		return false
	}
	for _, s := range slice {
		if strings.EqualFold(s, target) {
			return true
		}
	}
	return false
}

// IndexFold returns the index of target in slice (case-insensitive),
// or -1 if not present.
func IndexFold(slice []string, target string) int {
	for i, v := range slice {
		if strings.EqualFold(v, target) {
			return i
		}
	}
	return -1
}

// DropFold returns slice with all case-insensitive matches of target removed.
// The returned slice shares backing storage with the input; callers must not
// rely on the original contents after calling.
func DropFold(slice []string, target string) []string {
	out := slice[:0]
	for _, v := range slice {
		if !strings.EqualFold(v, target) {
			out = append(out, v)
		}
	}
	return out
}
