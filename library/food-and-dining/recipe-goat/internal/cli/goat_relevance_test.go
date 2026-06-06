package cli

import (
	"math"
	"testing"
)

func TestQueryRelevance(t *testing.T) {
	cases := []struct {
		title string
		query string
		want  float64
	}{
		// The original failure mode: "chocolate cake" returning chocolate
		// chip cookies at rank 1. Cookies match only "chocolate" → 0.5,
		// while a real chocolate cake matches both → 1.0. The 0.20-weight
		// relevance term is enough to flip the ranking.
		{"Broma Bakery's Best Chocolate Chip Cookies", "chocolate cake", 0.5},
		{"Classic Chocolate Layer Cake", "chocolate cake", 1.0},
		{"New York Cheesecake", "chocolate cake", 0.0},
		{"Tiramisu", "chocolate cake", 0.0},

		// Specific queries — relevance distinguishes near-matches from
		// exact matches without penalizing reasonable hits.
		{"Ultimate Chocolate Cake from Scratch", "moist chocolate layer cake from scratch", 0.6},
		{"Chicken Tikka Masala", "chicken tikka masala", 1.0},
		{"The Best Tiramisu Recipe", "tiramisu", 1.0},

		// Plural folding — "cookie" matches "cookies".
		{"Chocolate Chip Cookies", "best chocolate chip cookie", 1.0},

		// Stop words don't count toward query length.
		{"Apple Pie", "the best apple pie recipe", 1.0},

		// Empty / stop-words-only queries return neutral 0.5 so the
		// ranker doesn't penalize every recipe equally.
		{"Anything", "", 0.5},
		{"Anything", "the and of", 0.5},
	}

	for _, tc := range cases {
		got := queryRelevance(tc.title, tc.query)
		if math.Abs(got-tc.want) > 0.01 {
			t.Errorf("queryRelevance(%q, %q) = %.2f, want %.2f", tc.title, tc.query, got, tc.want)
		}
	}
}
