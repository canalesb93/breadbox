//go:build !lite

package service

import "testing"

// TestDecodeStrayUnicodeEscapes exercises the repair applied to agent-authored
// report prose: a model that emits the literal text of a backslash-u escape
// (instead of the rune it names) should have it decoded on ingestion.
//
// The escape inputs are assembled at runtime via u() from a lone backslash so
// the test source itself never contains a backslash-u sequence — Go (and the
// tools that author this file) would otherwise fold such a sequence into the
// very rune under test, making the cases pass vacuously.
func TestDecodeStrayUnicodeEscapes(t *testing.T) {
	bs := "\\" // a single backslash, kept out of every literal below
	u := func(hex string) string { return bs + "u" + hex }

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "em dash escape decoded",
			in:   "Routine review complete " + u("2014") + " cleared all 8 transactions",
			want: "Routine review complete — cleared all 8 transactions",
		},
		{
			name: "already-correct em dash untouched",
			in:   "Reviewed 47 transactions — no suspicious activity",
			want: "Reviewed 47 transactions — no suspicious activity",
		},
		{
			name: "no escape marker is a no-op",
			in:   "Weekly review complete",
			want: "Weekly review complete",
		},
		{
			name: "mixed en-dash and em-dash",
			in:   "range 10" + u("2013") + "20 " + u("2014") + " done",
			want: "range 10–20 — done",
		},
		{
			name: "curly quotes and ellipsis",
			in:   u("201c") + "Budget" + u("201d") + " alert" + u("2026"),
			want: "“Budget” alert…",
		},
		{
			name: "lowercase hex",
			in:   "caf" + u("00e9") + " spend",
			want: "café spend",
		},
		{
			name: "uppercase hex",
			in:   "caf" + u("00E9") + " spend",
			want: "café spend",
		},
		{
			name: "surrogate pair emoji",
			in:   "nice " + u("d83d") + u("de00") + " work",
			want: "nice 😀 work",
		},
		{
			name: "non-hex tail left literal",
			in:   "path" + u("nknown") + " segment",
			want: "path" + u("nknown") + " segment",
		},
		{
			name: "truncated escape left literal",
			in:   "abc " + bs + "u20",
			want: "abc " + bs + "u20",
		},
		{
			name: "unpaired high surrogate left literal",
			in:   "x " + u("d83d") + " y",
			want: "x " + u("d83d") + " y",
		},
		{
			name: "lone low surrogate left literal",
			in:   "x " + u("de00") + " y",
			want: "x " + u("de00") + " y",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decodeStrayUnicodeEscapes(c.in); got != c.want {
				t.Errorf("decodeStrayUnicodeEscapes(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
