package recipes

import (
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// SearchResult is a single Allrecipes search/category card surfaced to the user.
type SearchResult struct {
	URL         string  `json:"url"`
	RecipeID    string  `json:"recipeId,omitempty"`
	Slug        string  `json:"slug,omitempty"`
	Title       string  `json:"title"`
	Image       string  `json:"image,omitempty"`
	Rating      float64 `json:"rating,omitempty"`
	ReviewCount int     `json:"reviewCount,omitempty"`
}

// recipeURLRe matches Allrecipes recipe permalinks (//allrecipes.com/recipe/<id>/<slug>/).
// The capture groups extract the numeric ID and slug.
var recipeURLRe = regexp.MustCompile(`https?://(?:www\.)?allrecipes\.com/recipe/(\d+)/([a-z0-9-]+)/?`)

// cardBlockRe matches a search/category result card. Allrecipes wraps each card in
// an anchor tag whose href is the recipe permalink. The card body contains the
// title, image, and a "N Ratings" badge. We parse the card-by-card and pull the
// fields out of each block.
//
// The pattern captures: 1=href, 2=card body. Greedy on body-end is fine because
// closing </a> is the boundary.
var cardBlockRe = regexp.MustCompile(`(?is)<a[^>]+href="(https?://(?:www\.)?allrecipes\.com/recipe/\d+/[^"]+)"[^>]*>(.*?)</a>`)

// imgRe extracts the first <img src="..."> from a card body. We prefer src over
// data-src because Allrecipes lazy-loads with both, and the rendered (post-srv)
// HTML has src populated for above-the-fold cards.
var imgRe = regexp.MustCompile(`(?is)<img[^>]+src="([^"]+)"`)

// titleRe extracts the visible title. Allrecipes wraps it in
// <span class="card__title-text"> on most templates and <h3 class="card__title">
// on others. We try both and fall back to a simple alt-attribute scrape.
var titleSpanRe = regexp.MustCompile(`(?is)<span[^>]+class="[^"]*card__title-text[^"]*"[^>]*>([^<]+)</span>`)
var titleH3Re = regexp.MustCompile(`(?is)<h3[^>]+class="[^"]*card__title[^"]*"[^>]*>([^<]+)</h3>`)
var altRe = regexp.MustCompile(`(?is)<img[^>]+alt="([^"]+)"`)

// ratingRe pulls the "N Ratings" badge text. Allrecipes renders it as
// "<div class='mntl-recipe-card-meta__rating'>4.7 (2,040)</div>" or as
// "<span>2,040 Ratings</span>" depending on template. We try several shapes.
var ratingNumRe = regexp.MustCompile(`(?is)mntl-recipe-card-meta__rating[^>]*>\s*([0-9.]+)\s*\(([0-9,]+)\)`)
var reviewBadgeRe = regexp.MustCompile(`(?is)([0-9,]+)\s*Ratings`)

// ParseSearchResults extracts SearchResult records from an Allrecipes search or
// category results page. Returns up to `limit` results in DOM order.
//
// The parser is robust to template churn: if a field cannot be extracted, the
// surrounding fields still survive. A result is valid as long as URL + Title
// are populated; everything else is best-effort.
func ParseSearchResults(htmlBody []byte, limit int) []SearchResult {
	if limit <= 0 {
		limit = 24
	}
	out := []SearchResult{}
	seen := map[string]bool{}

	cards := cardBlockRe.FindAllSubmatch(htmlBody, -1)
	for _, m := range cards {
		href := string(m[1])
		// Drop URL fragments and query strings for dedup.
		canon := stripURLNoise(href)
		if seen[canon] {
			continue
		}
		seen[canon] = true

		body := m[2]
		sr := SearchResult{URL: canon}

		if rm := recipeURLRe.FindStringSubmatch(canon); rm != nil {
			sr.RecipeID = rm[1]
			sr.Slug = rm[2]
		}

		// Title: try span, then h3, then alt
		if t := titleSpanRe.FindSubmatch(body); t != nil {
			sr.Title = cleanText(string(t[1]))
		} else if t := titleH3Re.FindSubmatch(body); t != nil {
			sr.Title = cleanText(string(t[1]))
		} else if t := altRe.FindSubmatch(body); t != nil {
			sr.Title = cleanText(string(t[1]))
		}
		// If still empty, fall back to slug-derived title.
		if sr.Title == "" && sr.Slug != "" {
			sr.Title = slugToTitle(sr.Slug)
		}

		if im := imgRe.FindSubmatch(body); im != nil {
			sr.Image = string(im[1])
		}

		if rm := ratingNumRe.FindSubmatch(body); rm != nil {
			if v, err := strconv.ParseFloat(string(rm[1]), 64); err == nil {
				sr.Rating = v
			}
			sr.ReviewCount = parseCommaInt(string(rm[2]))
		} else if rm := reviewBadgeRe.FindSubmatch(body); rm != nil {
			sr.ReviewCount = parseCommaInt(string(rm[1]))
		}

		if sr.URL != "" && sr.Title != "" {
			out = append(out, sr)
		}
		if len(out) >= limit {
			break
		}
	}
	return out
}

// cleanText strips inner HTML tags and decodes entities to a single-line string.
func cleanText(s string) string {
	s = html.UnescapeString(s)
	s = stripTagsRe.ReplaceAllString(s, "")
	s = collapseWSRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

var stripTagsRe = regexp.MustCompile(`<[^>]+>`)
var collapseWSRe = regexp.MustCompile(`\s+`)

func stripURLNoise(u string) string {
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	return u
}

func parseCommaInt(s string) int {
	n, _ := strconv.Atoi(strings.ReplaceAll(s, ",", ""))
	return n
}

func slugToTitle(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// CanonicalRecipeURL returns the canonical permalink for an Allrecipes recipe
// given a recipe ID and slug. Used by the `recipe` command when called with an
// ID-only argument.
func CanonicalRecipeURL(id, slug string) string {
	if id == "" {
		return ""
	}
	if slug == "" {
		return "https://www.allrecipes.com/recipe/" + id + "/"
	}
	return "https://www.allrecipes.com/recipe/" + id + "/" + slug + "/"
}

// ResolveRecipeURL accepts either a full URL, a numeric ID, or an "id/slug"
// shorthand and returns a canonical recipe URL. Returns the input verbatim if
// it is already a non-Allrecipes URL (caller may still want to fetch).
func ResolveRecipeURL(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	// Already a URL?
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return input
	}
	// Numeric ID?
	if _, err := strconv.Atoi(input); err == nil {
		return CanonicalRecipeURL(input, "")
	}
	// id/slug shorthand?
	if i := strings.Index(input, "/"); i > 0 {
		return CanonicalRecipeURL(input[:i], input[i+1:])
	}
	return input
}

// ParseURL extracts the recipe ID and slug from an Allrecipes URL.
func ParseURL(u string) (id, slug string) {
	if m := recipeURLRe.FindStringSubmatch(u); m != nil {
		return m[1], m[2]
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return "", ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i, p := range parts {
		if p == "recipe" && i+2 < len(parts) {
			return parts[i+1], parts[i+2]
		}
	}
	return "", ""
}
