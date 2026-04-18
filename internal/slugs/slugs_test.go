package slugs

import "testing"

func TestTitleCase(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"single word lowercase", "review", "Review"},
		{"single word already capitalized", "Review", "Review"},
		{"hyphen separator", "needs-review", "Needs Review"},
		{"colon separator", "subscription:monthly", "Subscription Monthly"},
		{"underscore separator", "user_name", "User Name"},
		{"mixed separators", "needs-review:urgent_now", "Needs Review Urgent Now"},
		{"multiple words", "very-long-tag-name", "Very Long Tag Name"},
		{"leading separator", "-review", "Review"},
		{"trailing separator", "review-", "Review"},
		{"consecutive separators", "needs--review", "Needs Review"},
		{"all caps preserved past first char", "API-key", "API Key"},
		{"single character", "a", "A"},
		{"only separators", "---", ""},
		{"unicode word", "café-bar", "Café Bar"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TitleCase(tc.in)
			if got != tc.want {
				t.Errorf("TitleCase(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
