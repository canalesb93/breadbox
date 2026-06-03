//go:build !headless && !lite

package admin

import "testing"

func TestDecodeDataURL(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantBytes string
		wantCT    string
	}{
		{"data url jpeg", "data:image/jpeg;base64,SGk=", "Hi", "image/jpeg"},
		{"data url png", "data:image/png;base64,SGk=", "Hi", "image/png"},
		{"bare base64 defaults jpeg", "SGk=", "Hi", "image/jpeg"},
		{"empty", "", "", ""},
		{"whitespace", "   ", "", ""},
		{"bad base64", "data:image/jpeg;base64,!!!", "", ""},
		{"no comma", "data:image/jpeg;base64", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, ct := decodeDataURL(c.in)
			if string(data) != c.wantBytes {
				t.Errorf("bytes = %q, want %q", string(data), c.wantBytes)
			}
			if ct != c.wantCT {
				t.Errorf("content type = %q, want %q", ct, c.wantCT)
			}
		})
	}
}
