package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// validUUID is a well-formed UUID for testing parseUUID passthrough.
const validUUID = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"

// mockLookupOK returns a fixed UUID, simulating a successful short ID query.
func mockLookupOK(_ context.Context, _ string) (pgtype.UUID, error) {
	uid, _ := parseUUID(validUUID)
	return uid, nil
}

// mockLookupErr simulates a short ID lookup that finds no rows.
func mockLookupErr(_ context.Context, _ string) (pgtype.UUID, error) {
	return pgtype.UUID{}, fmt.Errorf("no rows")
}

func TestResolveID_ShortID_Found(t *testing.T) {
	s := &Service{}
	uid, err := s.resolveID(context.Background(), "AbCd1234", mockLookupOK, ErrNotFound)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !uid.Valid {
		t.Fatal("expected valid UUID")
	}
}

func TestResolveID_ShortID_NotFound(t *testing.T) {
	s := &Service{}
	_, err := s.resolveID(context.Background(), "AbCd1234", mockLookupErr, ErrNotFound)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestResolveID_ShortID_CustomError(t *testing.T) {
	s := &Service{}
	_, err := s.resolveID(context.Background(), "AbCd1234", mockLookupErr, ErrCategoryNotFound)
	if !errors.Is(err, ErrCategoryNotFound) {
		t.Fatalf("expected ErrCategoryNotFound, got: %v", err)
	}
}

func TestResolveID_ValidUUID(t *testing.T) {
	s := &Service{}
	// mockLookupErr should never be called for a UUID input.
	uid, err := s.resolveID(context.Background(), validUUID, mockLookupErr, ErrNotFound)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !uid.Valid {
		t.Fatal("expected valid UUID")
	}
}

func TestResolveID_InvalidInput(t *testing.T) {
	s := &Service{}
	_, err := s.resolveID(context.Background(), "not-a-uuid-or-short", mockLookupErr, ErrNotFound)
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatal("should not be ErrNotFound — should be an 'invalid id' error")
	}
}
