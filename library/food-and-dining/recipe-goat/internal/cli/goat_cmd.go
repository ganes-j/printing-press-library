package cli

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mvanhorn/printing-press-library/library/food-and-dining/recipe-goat/internal/recipes"

	"github.com/spf13/cobra"
)

// goatEntry is an internal ranking record. Exposed via JSON output.
type goatEntry struct {
	Rank        int     `json:"rank"`
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Site        string  `json:"site"`
	Author      string  `json:"author,omitempty"`
	Rating      float64 `json:"rating,omitempty"`
	ReviewCount int     `json:"reviewCount,omitempty"`
	TotalTimeS  int     `json:"totalTimeSeconds,omitempty"`
	Score       float64 `json:"score"`
}

func newGoatCmd(flags *rootFlags) *cobra.Command {
	var (
		limit    int
		sitesCSV string
		saveAll  bool
	)
	cmd := &cobra.Command{
		Use:         "goat <query>",
		Short:       "Cross-site recipe ranker — fetch candidates and rank by query relevance, rating, popularity, and site trust",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Search across curated recipe sites, fetch each candidate, then rank by
a weighted score of query-title overlap, rating, review volume, site
trust, and recency. Best for "I want the best <specific dish>" queries
("chicken tikka masala", "moist chocolate layer cake from scratch");
broad category queries like "chocolate cake" can still surface
popular-but-related recipes (cookies, cheesecake) — narrow with more
specific phrasing or --sites for category exploration.

Ranking weights:
  0.20 query_relevance + 0.35 rating_normalized
  + 0.25 log(reviews+1)/log(1000) + 0.15 site_trust + 0.05 recency_norm

query_relevance is the fraction of query tokens that appear in the
recipe title (case-insensitive, stop-word stripped, plural-folded).

Two trust-aware adjustments before scoring:
  - Editorial baseline: curated sites (trust >= 0.9) with no Schema.org
    rating get rating=4.5/reviews=100 imputed (treats curation as ~100
    implicit favorable reviews).
  - Bayesian smoothing: aggregator sites (trust < 0.85) ratings smoothed
    toward prior mean 4.0 with credibility C=200. A 5.0/100 becomes
    effective 4.33; a 4.7/5000 stays at 4.67.`,
		Example: "  recipe-goat-pp-cli goat \"chicken tikka masala\" --limit 5",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			ctx, cancel := context.WithTimeout(context.Background(), 2*flags.timeout)
			defer cancel()

			sites := siteHostsFromCSV(sitesCSV)
			if len(sites) == 0 {
				return usageErr(fmt.Errorf("no sites selected — check --sites"))
			}
			client := httpClientForSites(flags.timeout)

			// 1. Search each site concurrently.
			type searchOut struct {
				results []recipes.SearchResult
				site    recipes.Site
			}
			searchCh := make(chan searchOut, len(sites))
			var swg sync.WaitGroup
			sem := make(chan struct{}, 4)
			for _, s := range sites {
				s := s
				swg.Add(1)
				go func() {
					defer swg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					res, err := recipes.SearchSite(ctx, client, s, query, 3)
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "search %s: %v\n", s.Hostname, err)
						return
					}
					searchCh <- searchOut{results: res, site: s}
				}()
			}
			go func() { swg.Wait(); close(searchCh) }()

			var candidates []recipes.SearchResult
			for so := range searchCh {
				candidates = append(candidates, so.results...)
			}
			if len(candidates) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no candidates found (sites may be blocking unauthenticated scrapers — run `doctor` to check reachability)")
				if flags.asJSON {
					return flags.printJSON(cmd, []goatEntry{})
				}
				return nil
			}

			// 2. Fetch each candidate (4 workers) to get ratings/reviews/etc.
			// Only recipes that (a) parse successfully, (b) have ingredients
			// and a name, and (c) whose name matches at least one query token
			// make it into the ranking. This keeps editorial pages and
			// round-ups that slipped past the URL filter out of the results.
			type fetchOut struct {
				r    *recipes.Recipe
				src  recipes.SearchResult
				kept bool
			}
			fetchCh := make(chan fetchOut, len(candidates))
			var fwg sync.WaitGroup
			fsem := make(chan struct{}, 4)
			for _, c := range candidates {
				c := c
				fwg.Add(1)
				go func() {
					defer fwg.Done()
					fsem <- struct{}{}
					defer func() { <-fsem }()
					fctx, fcancel := context.WithTimeout(ctx, flags.timeout)
					defer fcancel()
					r, err := recipes.Fetch(fctx, client, c.URL)
					if err != nil || r == nil {
						fetchCh <- fetchOut{src: c, kept: false}
						return
					}
					// JSON-LD must include a name and at least one ingredient.
					if strings.TrimSpace(r.Name) == "" || len(r.RecipeIngredient) == 0 {
						fetchCh <- fetchOut{src: c, kept: false}
						return
					}
					// Title must match at least one query token.
					if !recipes.MatchesQueryPublic(r.Name, c.URL, query) {
						fetchCh <- fetchOut{src: c, kept: false}
						return
					}
					fetchCh <- fetchOut{r: r, src: c, kept: true}
				}()
			}
			go func() { fwg.Wait(); close(fetchCh) }()

			fetched := []*recipes.Recipe{}
			totalCandidates := 0
			for fo := range fetchCh {
				totalCandidates++
				if fo.kept && fo.r != nil {
					fetched = append(fetched, fo.r)
				}
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "filtered: kept %d of %d candidates\n", len(fetched), totalCandidates)

			// 3. Score.
			entries := make([]goatEntry, 0, len(fetched))
			for _, r := range fetched {
				site := recipes.FindSite(r.Site)
				score := goatScore(r, site, query)
				entries = append(entries, goatEntry{
					Title:       r.Name,
					URL:         r.URL,
					Site:        r.Site,
					Author:      r.Author,
					Rating:      r.AggregateRating.Value,
					ReviewCount: r.AggregateRating.Count,
					TotalTimeS:  r.TotalTime,
					Score:       score,
				})
			}
			sort.SliceStable(entries, func(i, j int) bool { return entries[i].Score > entries[j].Score })
			if limit > 0 && len(entries) > limit {
				entries = entries[:limit]
			}
			for i := range entries {
				entries[i].Rank = i + 1
			}

			// 4. Optionally save all.
			if saveAll && !flags.dryRun {
				st, err := openRecipeStore()
				if err == nil {
					defer st.Close()
					for _, r := range fetched {
						// Only save results with real JSON-LD (have ingredients).
						if len(r.RecipeIngredient) == 0 {
							continue
						}
						if _, err := st.SaveRecipe(recipeToStored(r)); err != nil {
							fmt.Fprintf(cmd.ErrOrStderr(), "save %s: %v\n", r.URL, err)
						}
					}
				}
			}

			if flags.asJSON {
				return flags.printJSON(cmd, entries)
			}
			headers := []string{"#", "TITLE", "SITE", "AUTHOR", "RATING", "REVIEWS", "TIME", "SCORE", "URL"}
			rows := make([][]string, 0, len(entries))
			for _, e := range entries {
				rating := "—"
				if e.Rating > 0 {
					rating = fmt.Sprintf("%.2f", e.Rating)
				}
				rows = append(rows, []string{
					strconv.Itoa(e.Rank),
					truncate(e.Title, 48),
					e.Site,
					truncate(e.Author, 18),
					rating,
					strconv.Itoa(e.ReviewCount),
					formatDuration(e.TotalTimeS),
					fmt.Sprintf("%.3f", e.Score),
					e.URL,
				})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 5, "Top N results to return")
	cmd.Flags().StringVar(&sitesCSV, "sites", "all", "Sites to query (CSV of hostnames, or 'all')")
	cmd.Flags().BoolVar(&saveAll, "save-all", false, "Save every fetched result to the cookbook")
	return cmd
}

// goatScore implements the ranking formula. All components are in [0,1].
//
// Query relevance is the fraction of query tokens (post-stopword,
// post-plural-fold) that appear in the recipe title. Without it the
// ranker becomes a popularity contest — a 5.0/552-review chocolate
// chip cookie outranks every chocolate cake when the query is
// "chocolate cake", because the keyword filter only requires one
// matching token.
//
// Two trust-aware adjustments before the weighted sum (added 2026-04-26):
//
//  1. Editorial-baseline imputation. When a curated site (trust >= 0.9)
//     publishes a recipe with NO Schema.org aggregateRating (rating=0,
//     reviews=0), impute rating=4.5 and reviews=100. Rationale: those
//     sites have human editorial vetting; the absence of a rating widget
//     is not the same as the absence of curation. This gives niche
//     curated recipes a fair floor without making them invincible.
//
//  2. Bayesian smoothing on aggregator-site ratings. When a crowdsourced
//     aggregator (trust < 0.85) reports a rating, smooth toward the prior
//     mean 4.0 with credibility weight C=200. A 5.0 with 100 reviews
//     becomes effective 4.33; a 4.7 with 5000 reviews stays 4.67. This
//     fixes the "100 reviewers gave it 5 stars" ≠ "5000 reviewers gave
//     it 4.7 stars" credibility problem the raw mean ignores.
func goatScore(r *recipes.Recipe, site recipes.Site, query string) float64 {
	rating := r.AggregateRating.Value
	reviews := r.AggregateRating.Count
	if rating == 0 && reviews == 0 && site.Trust >= 0.9 {
		rating = 4.5
		reviews = 100
	}
	if site.Trust < 0.85 && rating > 0 && reviews > 0 {
		const credibilityC = 200.0
		const priorMean = 4.0
		rating = (rating*float64(reviews) + priorMean*credibilityC) / (float64(reviews) + credibilityC)
	}

	ratingNorm := 0.0
	if rating > 0 {
		ratingNorm = rating / 5.0
		if ratingNorm > 1.0 {
			ratingNorm = 1.0
		}
	}
	reviewNorm := 0.0
	if reviews > 0 {
		reviewNorm = math.Log(float64(reviews+1)) / math.Log(1000)
		if reviewNorm > 1.0 {
			reviewNorm = 1.0
		}
	}
	siteTrust := site.Trust
	if siteTrust == 0 {
		siteTrust = 0.5
	}
	// Recency: newer = higher. Treat "recent" as last 5 years on a linear
	// scale; everything older gets 0.
	recency := 0.0
	if !r.FetchedAt.IsZero() {
		// FetchedAt is "now" effectively — use DatePublished if parseable.
		if r.DatePublished != "" {
			if t, err := time.Parse("2006-01-02", r.DatePublished[:min(10, len(r.DatePublished))]); err == nil {
				ageYears := time.Since(t).Hours() / 24 / 365
				recency = 1.0 - ageYears/5.0
				if recency < 0 {
					recency = 0
				}
				if recency > 1 {
					recency = 1
				}
			}
		}
	}
	relevance := queryRelevance(r.Name, query)
	return 0.20*relevance + 0.35*ratingNorm + 0.25*reviewNorm + 0.15*siteTrust + 0.05*recency
}

// queryRelevance returns the fraction of query tokens that appear in the
// recipe title. Title and query are tokenized identically: lowercased,
// punctuation stripped, English stop words removed, trailing 's' folded
// off so "cookie" and "cookies" match. An empty query (no tokens after
// filtering) returns 0.5 — neutral — so the ranker doesn't penalize
// every recipe equally when the user passes only stop words.
func queryRelevance(title, query string) float64 {
	qTokens := relevanceTokens(query)
	if len(qTokens) == 0 {
		return 0.5
	}
	titleSet := make(map[string]struct{}, 16)
	for _, t := range relevanceTokens(title) {
		titleSet[t] = struct{}{}
	}
	matched := 0
	for _, qt := range qTokens {
		if _, ok := titleSet[qt]; ok {
			matched++
		}
	}
	return float64(matched) / float64(len(qTokens))
}

var relevanceStopWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "the": {}, "of": {}, "for": {},
	"with": {}, "in": {}, "on": {}, "to": {}, "from": {}, "by": {},
	"recipe": {}, "recipes": {}, "best": {},
}

// relevanceTokens lowercases, splits on non-letter runes, drops short
// words and stop words, and strips a trailing 's' to fold simple plurals.
// Returns a slice (not a set) because callers may want positional info.
func relevanceTokens(s string) []string {
	s = strings.ToLower(s)
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) < 3 {
			continue
		}
		if _, stop := relevanceStopWords[f]; stop {
			continue
		}
		if len(f) > 3 && strings.HasSuffix(f, "s") {
			f = f[:len(f)-1]
		}
		out = append(out, f)
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
