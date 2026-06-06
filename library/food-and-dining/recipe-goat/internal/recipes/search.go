package recipes

import (
	"context"
	"html"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// SearchResult is a lightweight metadata record returned by site search.
// Fields come from the listing page — we do not fetch the recipe itself here.
type SearchResult struct {
	URL         string  `json:"url"`
	Title       string  `json:"title"`
	Site        string  `json:"site"`
	Rating      float64 `json:"rating,omitempty"`
	ReviewCount int     `json:"reviewCount,omitempty"`
	Author      string  `json:"author,omitempty"`
	TotalTime   int     `json:"totalTime,omitempty"` // seconds
}

// anchorRe matches <a href="..." ...>TEXT</a>. Greedy to grab full href.
var anchorRe = regexp.MustCompile(`(?is)<a\s+[^>]*href\s*=\s*["']([^"']+)["'][^>]*>(.*?)</a>`)

// SearchSite fetches the site's search URL for `query` and extracts up to
// `limit` candidate recipe links. This is a best-effort regex-based scrape:
// it targets anchor hrefs that look like recipe permalinks (using the site's
// RecipeURLPattern when available) and pulls the anchor text as the title.
// Returns (nil, nil) when no results — not an error.
func SearchSite(ctx context.Context, client *http.Client, site Site, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	u := BuildSearchURL(site, query)
	body, err := FetchHTML(ctx, client, u)
	if err != nil {
		return nil, err
	}
	return ExtractSearchResultsQuery(body, site, query, limit), nil
}

// ExtractSearchResults is the query-agnostic extractor. Callers that know the
// query should prefer ExtractSearchResultsQuery — it adds a title/URL token
// match check that filters editorial and round-up pages whose URL structure
// otherwise looks permalink-shaped.
func ExtractSearchResults(body []byte, site Site, limit int) []SearchResult {
	return ExtractSearchResultsQuery(body, site, "", limit)
}

// ExtractSearchResultsQuery parses anchors from already-fetched HTML and, when
// query != "", requires at least one query token to appear in either the
// anchor text or the URL path.
func ExtractSearchResultsQuery(body []byte, site Site, query string, limit int) []SearchResult {
	matches := anchorRe.FindAllSubmatch(body, -1)
	seen := map[string]bool{}
	out := []SearchResult{}
	for _, m := range matches {
		href := strings.TrimSpace(string(m[1]))
		text := cleanAnchorText(string(m[2]))
		if !looksLikeRecipeLink(href, site) {
			continue
		}
		abs := makeAbsoluteURL(href, site.Hostname)
		if abs == "" || seen[abs] {
			continue
		}
		if text == "" || len(text) < 3 {
			continue
		}
		// Filter navigation/header titles.
		lower := strings.ToLower(text)
		if strings.Contains(lower, "search results") || strings.Contains(lower, "subscribe") || strings.Contains(lower, "newsletter") || strings.Contains(lower, "log in") || strings.Contains(lower, "sign up") {
			continue
		}
		// Query relevance: require at least one token in title or URL slug.
		if query != "" && !matchesQuery(text, abs, query) {
			continue
		}
		seen[abs] = true
		out = append(out, SearchResult{
			URL:   abs,
			Title: text,
			Site:  site.Hostname,
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

// MatchesQueryPublic is the exported entry point to matchesQuery so callers
// outside this package (the goat command's JSON-LD validation layer) can
// share the same relevance logic.
func MatchesQueryPublic(title, urlStr, query string) bool {
	return matchesQuery(title, urlStr, query)
}

// matchesQuery returns true when at least one whitespace-separated token of
// query appears (case-insensitively) in either the title or the URL path.
// Tokens shorter than 3 chars are ignored — they are too generic.
func matchesQuery(title, urlStr, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	titleL := strings.ToLower(title)
	// Extract just the path for slug matching.
	path := urlStr
	if i := strings.Index(strings.ToLower(urlStr), "://"); i >= 0 {
		rest := urlStr[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			path = rest[j:]
		} else {
			path = "/"
		}
	}
	pathL := strings.ToLower(path)
	for _, tok := range strings.Fields(q) {
		tok = strings.TrimFunc(tok, func(r rune) bool {
			return r == '"' || r == '\'' || r == ',' || r == '.'
		})
		if len(tok) < 3 {
			continue
		}
		if strings.Contains(titleL, tok) || strings.Contains(pathL, tok) {
			return true
		}
		// Singular/plural tolerance: "brownies" vs "brownie".
		if strings.HasSuffix(tok, "s") && len(tok) > 3 {
			stem := tok[:len(tok)-1]
			if strings.Contains(titleL, stem) || strings.Contains(pathL, stem) {
				return true
			}
		}
	}
	return false
}

var tagRe = regexp.MustCompile(`<[^>]+>`)
var spaceRe = regexp.MustCompile(`\s+`)

// ratingNoiseRe strips common search-card noise that sites bolt onto the
// visible anchor text: rating counts ("219 Ratings"), per-serving prices
// ("$4.32 recipe / $0.18 serving"), and descriptor tails with ellipsis.
var ratingNoiseRe = regexp.MustCompile(`(?i)\s*\d+\s*(ratings?|reviews?|stars?|comments?)\b.*$`)
var priceNoiseRe = regexp.MustCompile(`\s*\$\d+(\.\d+)?\s*(recipe|per\s+serving|/\s*serv).*$`)
var trailingDescriptionRe = regexp.MustCompile(`\.\s+[A-Z][^.]{40,}$`)

func cleanAnchorText(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = spaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	// Strip common search-card noise.
	s = ratingNoiseRe.ReplaceAllString(s, "")
	s = priceNoiseRe.ReplaceAllString(s, "")
	s = trailingDescriptionRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// looksLikeRecipeLink decides whether an anchor href on a search-result page
// is likely to be a real recipe permalink. The previous implementation used a
// permissive "slug has a dash" fallback that pulled in category pages, tag
// index pages, and editorial round-ups. The replacement checks:
//
//  1. Fast-fail rejections (mailto:, javascript:, image/asset extensions,
//     /tag/, /category/, /search, /about, etc.).
//  2. Same-host requirement.
//  3. If the site has a RecipeURLPattern (configured in sites.go), require the
//     URL's *path* to match it. Otherwise fall back to "path contains /recipe"
//     so that unknown hosts (e.g., trending scraping) still behave reasonably.
func looksLikeRecipeLink(href string, site Site) bool {
	if href == "" {
		return false
	}
	l := strings.ToLower(href)
	if strings.HasPrefix(l, "#") || strings.HasPrefix(l, "mailto:") || strings.HasPrefix(l, "javascript:") {
		return false
	}
	// Skip obvious non-recipe paths.
	for _, bad := range []string{"/search", "/tag/", "/category/", "/categories/", "/about", "/contact", "/privacy", "/subscribe", "/newsletter", "/author/", "/authors/", "/page/", "/collection/", "/collections/", "/topics/", "/gallery/", "/galleries/", "/video/", "/videos/", "/shop", "/store/", "/random", "/feed", "/feeds", "/recommends/", "/recommend/", ".jpg", ".png", ".webp", ".gif", ".svg", ".pdf", ".css", ".js"} {
		if strings.Contains(l, bad) {
			return false
		}
	}
	// Same-host absolute OR root-relative.
	host := site.Hostname
	if strings.HasPrefix(l, "http") {
		if host != "" && !strings.Contains(l, host) {
			return false
		}
	} else if !strings.HasPrefix(l, "/") {
		return false
	}
	// Extract just the path portion.
	path := l
	if strings.HasPrefix(path, "http") {
		if i := strings.Index(path[8:], "/"); i >= 0 {
			path = path[8+i:]
		} else {
			path = "/"
		}
	}
	// Strip query/fragment.
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}

	// Per-site pattern is the gating filter when set.
	if site.RecipeURLPattern != nil {
		return site.RecipeURLPattern.MatchString(path)
	}
	// Fallback for unknown hosts: accept only paths that clearly point at a
	// single recipe (no longer accepting "anything with a dash in the slug").
	if strings.Contains(path, "/recipe/") || strings.Contains(path, "/recipes/") {
		segs := strings.Split(strings.Trim(path, "/"), "/")
		last := segs[len(segs)-1]
		// Reject bare /recipes/ index and /recipes/collection/... style pages.
		if last == "" || last == "recipe" || last == "recipes" {
			return false
		}
		return strings.Contains(last, "-") && len(last) > 6
	}
	return false
}

func makeAbsoluteURL(href, host string) string {
	if strings.HasPrefix(strings.ToLower(href), "http") {
		return href
	}
	if strings.HasPrefix(href, "//") {
		return "https:" + href
	}
	if strings.HasPrefix(href, "/") {
		return "https://www." + host + href
	}
	return ""
}

// SearchAll fans out concurrently across all sites (4 workers) and aggregates
// results. Per-site errors are swallowed — best effort. Deduplicated by URL.
func SearchAll(ctx context.Context, client *http.Client, query string, limit int) ([]SearchResult, error) {
	type job struct {
		site Site
	}
	jobs := make(chan job, len(Sites))
	for _, s := range Sites {
		jobs <- job{site: s}
	}
	close(jobs)

	var mu sync.Mutex
	all := []SearchResult{}

	var wg sync.WaitGroup
	workers := 4
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				res, err := SearchSite(ctx, client, j.site, query, limit)
				if err != nil || len(res) == 0 {
					continue
				}
				mu.Lock()
				all = append(all, res...)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	// Dedupe.
	seen := map[string]bool{}
	out := make([]SearchResult, 0, len(all))
	for _, r := range all {
		if seen[r.URL] {
			continue
		}
		seen[r.URL] = true
		out = append(out, r)
	}
	// Sort stable by site then title for deterministic output.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Site != out[j].Site {
			return out[i].Site < out[j].Site
		}
		return out[i].Title < out[j].Title
	})
	return out, nil
}
