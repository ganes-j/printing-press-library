package cli

import (
	"testing"
)

func TestNameMatchesAtWordBoundary(t *testing.T) {
	cases := []struct {
		name     string
		haystack string
		needle   string
		want     bool
	}{
		{
			name:     "exact match",
			haystack: "Patrick Collison",
			needle:   "Patrick Collison",
			want:     true,
		},
		{
			name:     "case insensitive",
			haystack: "patrick collison",
			needle:   "Patrick Collison",
			want:     true,
		},
		{
			name:     "name embedded in longer text",
			haystack: "Stripe was founded by Patrick Collison and John Collison.",
			needle:   "Patrick Collison",
			want:     true,
		},
		{
			name:     "substring without word boundary should not match",
			haystack: "Patrick Collinsworth",
			needle:   "Patrick Collins",
			want:     false,
		},
		{
			name:     "first-name-only is not a substring match",
			haystack: "Patrick Smith",
			needle:   "Patrick Collison",
			want:     false,
		},
		{
			name:     "multiple spaces between words",
			haystack: "Patrick   Collison filed",
			needle:   "Patrick Collison",
			want:     true,
		},
		{
			name:     "single word match at word boundary",
			haystack: "Anthropic, PBC",
			needle:   "Anthropic",
			want:     true,
		},
		{
			name:     "single word substring at word edge does not match",
			haystack: "Anthropics Inc.",
			needle:   "Anthropic",
			want:     false,
		},
		{
			name:     "empty needle",
			haystack: "Patrick Collison",
			needle:   "",
			want:     false,
		},
		{
			name:     "empty haystack",
			haystack: "",
			needle:   "Patrick Collison",
			want:     false,
		},
		{
			name:     "punctuation around match",
			haystack: "(Patrick Collison)",
			needle:   "Patrick Collison",
			want:     true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nameMatchesAtWordBoundary(tc.haystack, tc.needle)
			if got != tc.want {
				t.Errorf("nameMatchesAtWordBoundary(%q, %q) = %v, want %v", tc.haystack, tc.needle, got, tc.want)
			}
		})
	}
}

func TestMentionTotalNilSafe(t *testing.T) {
	if got := mentionTotal(nil); got != 0 {
		t.Errorf("mentionTotal(nil) = %d, want 0", got)
	}
	m := &fundingMentions{Total: 7}
	if got := mentionTotal(m); got != 7 {
		t.Errorf("mentionTotal(%+v) = %d, want 7", m, got)
	}
}
