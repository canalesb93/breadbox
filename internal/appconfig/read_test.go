package appconfig

import (
	"context"
	"errors"
	"testing"

	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
)

type stubReader struct {
	rows map[string]db.AppConfig
	err  error
}

func (s stubReader) GetAppConfig(_ context.Context, key string) (db.AppConfig, error) {
	if s.err != nil {
		return db.AppConfig{}, s.err
	}
	row, ok := s.rows[key]
	if !ok {
		return db.AppConfig{}, errors.New("not found")
	}
	return row, nil
}

func makeRow(value string, valid bool) db.AppConfig {
	return db.AppConfig{
		Key:   "k",
		Value: pgtype.Text{String: value, Valid: valid},
	}
}

func TestRead(t *testing.T) {
	ctx := context.Background()
	r := stubReader{rows: map[string]db.AppConfig{
		"present": makeRow("abc", true),
		"null":    makeRow("", false),
		"empty":   makeRow("", true),
	}}

	if v, ok := Read(ctx, r, "present"); !ok || v != "abc" {
		t.Errorf("Read(present) = (%q, %v), want (abc, true)", v, ok)
	}
	if _, ok := Read(ctx, r, "null"); ok {
		t.Error("Read(null) should return ok=false for NULL value")
	}
	if v, ok := Read(ctx, r, "empty"); !ok || v != "" {
		t.Errorf("Read(empty) = (%q, %v), want (\"\", true)", v, ok)
	}
	if _, ok := Read(ctx, r, "missing"); ok {
		t.Error("Read(missing) should return ok=false for missing key")
	}
	if _, ok := Read(ctx, stubReader{err: errors.New("boom")}, "x"); ok {
		t.Error("Read should return ok=false when query errors")
	}
}

func TestString(t *testing.T) {
	ctx := context.Background()
	r := stubReader{rows: map[string]db.AppConfig{
		"set":   makeRow("abc", true),
		"empty": makeRow("", true),
		"null":  makeRow("", false),
	}}

	cases := []struct {
		key, def, want string
	}{
		{"set", "D", "abc"},
		{"empty", "D", "D"},
		{"null", "D", "D"},
		{"missing", "D", "D"},
	}
	for _, c := range cases {
		if got := String(ctx, r, c.key, c.def); got != c.want {
			t.Errorf("String(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}

func TestBool(t *testing.T) {
	ctx := context.Background()
	r := stubReader{rows: map[string]db.AppConfig{
		"true_val":  makeRow("true", true),
		"false_val": makeRow("false", true),
		"junk":      makeRow("yes", true),
		"empty":     makeRow("", true),
		"null":      makeRow("", false),
	}}

	cases := []struct {
		key  string
		def  bool
		want bool
	}{
		{"true_val", false, true},
		{"false_val", true, false},
		{"junk", true, false},
		{"empty", true, false},
		{"null", true, true},
		{"missing", false, false},
		{"missing", true, true},
	}
	for _, c := range cases {
		if got := Bool(ctx, r, c.key, c.def); got != c.want {
			t.Errorf("Bool(%q, def=%v) = %v, want %v", c.key, c.def, got, c.want)
		}
	}
}

func TestInt(t *testing.T) {
	ctx := context.Background()
	r := stubReader{rows: map[string]db.AppConfig{
		"int":    makeRow("42", true),
		"zero":   makeRow("0", true),
		"neg":    makeRow("-5", true),
		"junk":   makeRow("abc", true),
		"empty":  makeRow("", true),
		"null":   makeRow("", false),
	}}

	cases := []struct {
		key  string
		def  int
		want int
	}{
		{"int", 7, 42},
		{"zero", 7, 0},
		{"neg", 7, -5},
		{"junk", 7, 7},
		{"empty", 7, 7},
		{"null", 7, 7},
		{"missing", 7, 7},
	}
	for _, c := range cases {
		if got := Int(ctx, r, c.key, c.def); got != c.want {
			t.Errorf("Int(%q, def=%d) = %d, want %d", c.key, c.def, got, c.want)
		}
	}
}
