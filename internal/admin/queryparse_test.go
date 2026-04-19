package admin

import (
	"net/url"
	"testing"
	"time"
)

func TestOptStrQuery(t *testing.T) {
	q := url.Values{"a": {"hello"}, "b": {""}}

	if got := optStrQuery(q, "a"); got == nil || *got != "hello" {
		t.Errorf("present key: got %v, want pointer to 'hello'", got)
	}
	if got := optStrQuery(q, "b"); got != nil {
		t.Errorf("empty value: got %v, want nil", got)
	}
	if got := optStrQuery(q, "missing"); got != nil {
		t.Errorf("absent key: got %v, want nil", got)
	}
}

func TestOptDateQuery(t *testing.T) {
	want := time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC)

	cases := map[string]struct {
		value string
		want  *time.Time
	}{
		"valid date":      {"2026-04-19", &want},
		"empty":           {"", nil},
		"malformed":       {"not-a-date", nil},
		"wrong format":    {"04/19/2026", nil},
		"datetime string": {"2026-04-19T00:00:00Z", nil},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			q := url.Values{"d": {tc.value}}
			got := optDateQuery(q, "d")
			if tc.want == nil {
				if got != nil {
					t.Errorf("got %v, want nil", got)
				}
				return
			}
			if got == nil || !got.Equal(*tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}

	// Absent key behaves the same as empty.
	if got := optDateQuery(url.Values{}, "d"); got != nil {
		t.Errorf("absent key: got %v, want nil", got)
	}
}

func TestOptEndDateQuery(t *testing.T) {
	q := url.Values{"d": {"2026-04-19"}}
	got := optEndDateQuery(q, "d")
	want := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	if got == nil || !got.Equal(want) {
		t.Errorf("got %v, want %v (one day after input)", got, want)
	}

	if got := optEndDateQuery(url.Values{"d": {"bogus"}}, "d"); got != nil {
		t.Errorf("malformed value: got %v, want nil", got)
	}
	if got := optEndDateQuery(url.Values{}, "d"); got != nil {
		t.Errorf("absent key: got %v, want nil", got)
	}
}

func TestOptFloatQuery(t *testing.T) {
	cases := map[string]struct {
		value string
		want  *float64
	}{
		"integer":   {"42", ptrFloat(42)},
		"decimal":   {"12.5", ptrFloat(12.5)},
		"negative":  {"-3.14", ptrFloat(-3.14)},
		"empty":     {"", nil},
		"malformed": {"abc", nil},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			q := url.Values{"f": {tc.value}}
			got := optFloatQuery(q, "f")
			if tc.want == nil {
				if got != nil {
					t.Errorf("got %v, want nil", got)
				}
				return
			}
			if got == nil || *got != *tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func ptrFloat(f float64) *float64 { return &f }
