package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"breadbox/internal/service"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestErrorCode_Mapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"not found", service.ErrNotFound, CodeNotFound},
		{"category not found", service.ErrCategoryNotFound, CodeNotFound},
		{"forbidden", service.ErrForbidden, CodeForbidden},
		{"invalid cursor", service.ErrInvalidCursor, CodeInvalidCursor},
		{"invalid parameter", service.ErrInvalidParameter, CodeInvalidParameter},
		{"sync in progress", service.ErrSyncInProgress, CodeSyncInProgress},
		{"slug conflict", service.ErrSlugConflict, CodeSlugConflict},
		{"category undeletable", service.ErrCategoryUndeletable, CodeCategoryUndeletable},
		{"unknown error", errors.New("something blew up"), CodeInternalError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ErrorCode(tc.err); got != tc.want {
				t.Fatalf("ErrorCode(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

func TestErrorCode_WrappedError(t *testing.T) {
	// errors.Is should unwrap fmt.Errorf chains.
	wrapped := fmt.Errorf("failed to load category: %w", service.ErrCategoryNotFound)
	if got := ErrorCode(wrapped); got != CodeNotFound {
		t.Fatalf("ErrorCode(wrapped ErrCategoryNotFound) = %q, want %q", got, CodeNotFound)
	}

	deeper := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", service.ErrCategoryNotFound))
	if got := ErrorCode(deeper); got != CodeNotFound {
		t.Fatalf("ErrorCode(deeply wrapped ErrCategoryNotFound) = %q, want %q", got, CodeNotFound)
	}
}

func TestErrorResult_EnvelopeShape(t *testing.T) {
	res := errorResult(service.ErrCategoryNotFound)
	if !res.IsError {
		t.Fatal("expected IsError=true")
	}
	if len(res.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(tc.Text), &payload); err != nil {
		t.Fatalf("envelope is not valid JSON: %v. raw=%q", err, tc.Text)
	}
	if payload["code"] != CodeNotFound {
		t.Fatalf("code = %q, want %q", payload["code"], CodeNotFound)
	}
	if payload["error"] != service.ErrCategoryNotFound.Error() {
		t.Fatalf("error message = %q, want %q", payload["error"], service.ErrCategoryNotFound.Error())
	}
}

func TestErrorResult_UnknownErrorGetsInternalCode(t *testing.T) {
	res := errorResult(errors.New("surprise"))
	tc := res.Content[0].(*mcpsdk.TextContent)
	var payload map[string]string
	if err := json.Unmarshal([]byte(tc.Text), &payload); err != nil {
		t.Fatalf("envelope is not valid JSON: %v", err)
	}
	if payload["code"] != CodeInternalError {
		t.Fatalf("unknown err should get INTERNAL_ERROR, got %q", payload["code"])
	}
	if payload["error"] != "surprise" {
		t.Fatalf("error message = %q, want %q", payload["error"], "surprise")
	}
}

func TestErrorResultWithCode_DirectBuilder(t *testing.T) {
	res := errorResultWithCode(CodeInvalidParameter, "search_mode must be contains, words, or fuzzy")
	if !res.IsError {
		t.Fatal("expected IsError=true")
	}
	tc := res.Content[0].(*mcpsdk.TextContent)
	var payload map[string]string
	if err := json.Unmarshal([]byte(tc.Text), &payload); err != nil {
		t.Fatalf("envelope is not valid JSON: %v", err)
	}
	if payload["code"] != CodeInvalidParameter {
		t.Fatalf("code = %q, want %q", payload["code"], CodeInvalidParameter)
	}
	if payload["error"] != "search_mode must be contains, words, or fuzzy" {
		t.Fatalf("unexpected message: %q", payload["error"])
	}
}
