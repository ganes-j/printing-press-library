package recipes

// BayesianRating returns the Bayesian-smoothed rating for a recipe.
//
//	smoothed = (C * priorMean + reviewCount * rating) / (C + reviewCount)
//
// C is the credibility weight: a higher C demands more reviews before a recipe
// can pull away from the prior. priorMean is the corpus prior (4.0 is sensible
// for Allrecipes, where most rated recipes cluster between 4.2 and 4.7).
//
// This is a flat Bayesian shrinkage estimator, not the full hierarchical model
// IMDb uses. It's strong enough to crush 5-star/1-review noise without being a
// black box.
func BayesianRating(rating float64, reviewCount int, priorMean float64, c int) float64 {
	if c <= 0 {
		return rating
	}
	cf := float64(c)
	rc := float64(reviewCount)
	if rating <= 0 {
		return priorMean
	}
	return (cf*priorMean + rc*rating) / (cf + rc)
}

// Rank sorts SearchResult records by Bayesian-smoothed rating in descending
// order. Stable sort: ties preserve input order.
func Rank(results []SearchResult, priorMean float64, c int) []SearchResult {
	if len(results) <= 1 {
		return results
	}
	scored := make([]rankedResult, len(results))
	for i, r := range results {
		scored[i] = rankedResult{
			r:     r,
			score: BayesianRating(r.Rating, r.ReviewCount, priorMean, c),
		}
	}
	stableSortRanked(scored)
	out := make([]SearchResult, len(scored))
	for i, s := range scored {
		out[i] = s.r
	}
	return out
}

type rankedResult struct {
	r     SearchResult
	score float64
}

// stableSortRanked sorts in descending score order. Insertion sort is fine —
// search returns at most 24 results per page.
func stableSortRanked(s []rankedResult) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].score > s[j-1].score; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
