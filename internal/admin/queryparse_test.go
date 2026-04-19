package admin

import (
	"net/url"
	"testing"
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
