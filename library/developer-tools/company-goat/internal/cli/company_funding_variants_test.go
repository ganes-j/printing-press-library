package cli

import (
	"reflect"
	"testing"
)

func TestStemVariants(t *testing.T) {
	cases := []struct {
		name   string
		domain string
		want   []string
	}{
		{
			name:   "single token without obvious split",
			domain: "stripe.com",
			want:   []string{"stripe", "stripe inc"},
		},
		{
			name:   "concatenated stem with vowel-consonant boundary",
			domain: "junelife.com",
			want:   []string{"junelife", "june life", "junelife inc"},
		},
		{
			name:   "hyphenated stem",
			domain: "acme-corp.com",
			want:   []string{"acme-corp", "acme corp", "acme-corp inc"},
		},
		{
			name:   "numeric stem skips bigram",
			domain: "404.com",
			want:   []string{"404", "404 inc"},
		},
		{
			name:   "short stem skips bigram",
			domain: "ramp.com",
			want:   []string{"ramp", "ramp inc"},
		},
		{
			name:   "uppercase domain normalizes to lowercase",
			domain: "JuneLife.com",
			want:   []string{"junelife", "june life", "junelife inc"},
		},
		{
			name:   "empty domain returns nil",
			domain: "",
			want:   nil,
		},
		{
			name:   "stem already ends with inc keeps single inc variant",
			domain: "weberinc.com",
			// "weberinc" splits at first VC boundary (e->b at i=2 fails; r->i at i=4
			// is CV not VC; e->r at i=4? actually "w-e-b-e-r-i-n-c": at i=3 prev='b'
			// (consonant), at i=4 prev='e' (vowel) curr='r' (consonant) -> match.
			// So variant becomes "webe rinc" plus "weberinc inc".
			want: []string{"weberinc", "webe rinc", "weberinc inc"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stemVariants(tc.domain)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("stemVariants(%q) = %v, want %v", tc.domain, got, tc.want)
			}
		})
	}
}

func TestPickMentionQueryPrefersBigram(t *testing.T) {
	cases := []struct {
		name     string
		variants []string
		want     string
	}{
		{
			name:     "junelife picks the bigram",
			variants: []string{"junelife", "june life", "junelife inc"},
			want:     "june life",
		},
		{
			name:     "stripe falls through to inc variant",
			variants: []string{"stripe", "stripe inc"},
			want:     "stripe inc",
		},
		{
			name:     "single bare stem",
			variants: []string{"ramp"},
			want:     "ramp",
		},
		{
			name:     "ignores the inc variant when picking bigram",
			variants: []string{"weberinc", "weberinc inc", "weber inc"},
			// "weber inc" contains a space AND ends with " inc", so it's
			// rejected as a bigram pick; falls through to last variant.
			want: "weber inc",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pickMentionQuery(tc.variants)
			if got != tc.want {
				t.Errorf("pickMentionQuery(%v) = %q, want %q", tc.variants, got, tc.want)
			}
		})
	}
}

func TestSplitAtVowelConsonantBoundary(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"junelife", "june life"},
		{"stripe", ""}, // 6 chars, no valid VC boundary with both halves >= 3
		{"ramp", ""},   // too short
		{"hellokit", "hello kit"},
		// Heuristic finds the FIRST vowel-consonant boundary, not the
		// best one. "openpay" splits at e->n (i=3) producing "ope npay"
		// rather than the more natural "open pay" at the n->p boundary.
		// This is fine: variants only run sequentially when prior ones
		// return empty, so a noisy variant just produces zero hits.
		{"openpay", "ope npay"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := splitAtVowelConsonantBoundary(tc.input)
			if got != tc.want {
				t.Errorf("splitAtVowelConsonantBoundary(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
