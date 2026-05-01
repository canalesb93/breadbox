package mcp

// Unit tests for the template URI parser. No DB needed — these stay outside
// the integration build tag so they run in `go test ./...`.

import "testing"

func TestExtractTemplateParam(t *testing.T) {
	cases := []struct {
		name    string
		uri     string
		prefix  string
		want    string
		wantErr bool
	}{
		{
			name:   "happy path",
			uri:    "breadbox://transaction/abc12345",
			prefix: "breadbox://transaction/",
			want:   "abc12345",
		},
		{
			name:   "percent-encoded decodes",
			uri:    "breadbox://transaction/abc%20def",
			prefix: "breadbox://transaction/",
			want:   "abc def",
		},
		{
			name:    "empty tail",
			uri:     "breadbox://transaction/",
			prefix:  "breadbox://transaction/",
			wantErr: true,
		},
		{
			name:    "wrong prefix",
			uri:     "breadbox://account/abc",
			prefix:  "breadbox://transaction/",
			wantErr: true,
		},
		{
			name:    "extra path segment",
			uri:     "breadbox://transaction/abc/extra",
			prefix:  "breadbox://transaction/",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractTemplateParam(tc.uri, tc.prefix)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
