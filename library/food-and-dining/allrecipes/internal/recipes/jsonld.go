// Package recipes implements cross-site recipe aggregation: JSON-LD parsing,
// site registry, concurrent search, ingredient scaling, and substitution
// lookup. Used by the top-level CLI commands (recipe, goat, cookbook, etc.).
package recipes

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Recipe is the normalized in-memory representation of a parsed recipe.
// It is a superset of schema.org/Recipe fields plus provenance metadata.
type Recipe struct {
	URL                string            `json:"url"`
	Name               string            `json:"name"`
	Author             string            `json:"author,omitempty"`
	Image              string            `json:"image,omitempty"`
	RecipeIngredient   []string          `json:"recipeIngredient,omitempty"`
	RecipeInstructions []string          `json:"recipeInstructions,omitempty"`
	PrepTime           int               `json:"prepTime,omitempty"`
	CookTime           int               `json:"cookTime,omitempty"`
	TotalTime          int               `json:"totalTime,omitempty"`
	RecipeYield        string            `json:"recipeYield,omitempty"`
	AggregateRating    AggregateRating   `json:"aggregateRating,omitempty"`
	Nutrition          map[string]string `json:"nutrition,omitempty"`
	DatePublished      string            `json:"datePublished,omitempty"`
	Description        string            `json:"description,omitempty"`
	Keywords           []string          `json:"keywords,omitempty"`
	RecipeCategory     []string          `json:"recipeCategory,omitempty"`
	RecipeCuisine      []string          `json:"recipeCuisine,omitempty"`
	Site               string            `json:"site,omitempty"`
	FetchedAt          time.Time         `json:"fetchedAt,omitempty"`
}

// AggregateRating is a condensed view of schema.org/AggregateRating.
type AggregateRating struct {
	Value float64 `json:"value,omitempty"`
	Count int     `json:"count,omitempty"`
}

// ErrNoJSONLD is returned by ParseJSONLD when the HTML has no Recipe node.
var ErrNoJSONLD = errors.New("no JSON-LD Recipe found")

// jsonLDRe matches <script type="application/ld+json">...</script> blocks.
// Flags: (?is) = case-insensitive + dotall.
var jsonLDRe = regexp.MustCompile(`(?is)<script[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)

// ParseJSONLD extracts a Recipe from HTML by scanning JSON-LD blocks.
// It handles single Recipe objects, @graph arrays, and top-level arrays,
// looking for any node with @type == "Recipe".
func ParseJSONLD(htmlBody []byte, sourceURL string) (*Recipe, error) {
	matches := jsonLDRe.FindAllSubmatch(htmlBody, -1)
	for _, m := range matches {
		raw := bytes_trim(m[1])
		node := findRecipeNode(raw)
		if node != nil {
			r := recipeFromNode(node, sourceURL)
			return r, nil
		}
	}
	return nil, ErrNoJSONLD
}

func bytes_trim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

// findRecipeNode walks a JSON-LD blob looking for a Recipe node. The blob may
// be: a single object, an array, or an object with @graph. Returns the raw
// map if found, nil otherwise.
func findRecipeNode(raw []byte) map[string]any {
	var single map[string]any
	if err := json.Unmarshal(raw, &single); err == nil {
		if n := walkForRecipe(single); n != nil {
			return n
		}
	}
	var array []map[string]any
	if err := json.Unmarshal(raw, &array); err == nil {
		for _, item := range array {
			if n := walkForRecipe(item); n != nil {
				return n
			}
		}
	}
	return nil
}

// walkForRecipe returns the node if it is a Recipe, or descends into @graph.
func walkForRecipe(node map[string]any) map[string]any {
	if isRecipeType(node["@type"]) {
		return node
	}
	if graph, ok := node["@graph"].([]any); ok {
		for _, g := range graph {
			if gm, ok := g.(map[string]any); ok {
				if r := walkForRecipe(gm); r != nil {
					return r
				}
			}
		}
	}
	return nil
}

func isRecipeType(t any) bool {
	switch v := t.(type) {
	case string:
		return v == "Recipe"
	case []any:
		for _, x := range v {
			if s, ok := x.(string); ok && s == "Recipe" {
				return true
			}
		}
	}
	return false
}

func recipeFromNode(n map[string]any, sourceURL string) *Recipe {
	r := &Recipe{
		URL:       sourceURL,
		FetchedAt: time.Now().UTC(),
	}
	if u, _ := url.Parse(sourceURL); u != nil {
		r.Site = strings.TrimPrefix(u.Hostname(), "www.")
	}
	r.Name = asString(n["name"])
	if r.Name == "" {
		r.Name = asString(n["headline"])
	}
	r.Author = extractAuthor(n["author"])
	r.Image = extractImage(n["image"])
	r.Description = asString(n["description"])
	r.DatePublished = asString(n["datePublished"])
	r.RecipeYield = extractYield(n["recipeYield"])

	r.RecipeIngredient = extractStringList(n["recipeIngredient"])
	if len(r.RecipeIngredient) == 0 {
		r.RecipeIngredient = extractStringList(n["ingredients"])
	}
	r.RecipeInstructions = CleanInstructions(n["recipeInstructions"])

	r.PrepTime = ParseISO8601Duration(asString(n["prepTime"]))
	r.CookTime = ParseISO8601Duration(asString(n["cookTime"]))
	r.TotalTime = ParseISO8601Duration(asString(n["totalTime"]))
	if r.TotalTime == 0 && (r.PrepTime > 0 || r.CookTime > 0) {
		r.TotalTime = r.PrepTime + r.CookTime
	}

	r.AggregateRating = extractRating(n["aggregateRating"])
	r.Nutrition = extractNutrition(n["nutrition"])
	r.Keywords = extractKeywords(n["keywords"])
	r.RecipeCategory = extractStringList(n["recipeCategory"])
	r.RecipeCuisine = extractStringList(n["recipeCuisine"])
	return r
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return html.UnescapeString(strings.TrimSpace(x))
	case []any:
		if len(x) > 0 {
			return asString(x[0])
		}
	case map[string]any:
		if s, ok := x["@value"].(string); ok {
			return html.UnescapeString(s)
		}
		if s, ok := x["name"].(string); ok {
			return html.UnescapeString(s)
		}
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	}
	return ""
}

func extractAuthor(v any) string {
	switch x := v.(type) {
	case string:
		return html.UnescapeString(strings.TrimSpace(x))
	case map[string]any:
		return asString(x["name"])
	case []any:
		names := []string{}
		for _, a := range x {
			s := extractAuthor(a)
			if s != "" {
				names = append(names, s)
			}
		}
		return strings.Join(names, ", ")
	}
	return ""
}

func extractImage(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []any:
		if len(x) > 0 {
			return extractImage(x[0])
		}
	case map[string]any:
		if s, ok := x["url"].(string); ok {
			return s
		}
		return asString(x["@id"])
	}
	return ""
}

func extractYield(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.Itoa(int(x))
	case int:
		return strconv.Itoa(x)
	case []any:
		if len(x) > 0 {
			return extractYield(x[0])
		}
	}
	return ""
}

func extractStringList(v any) []string {
	switch x := v.(type) {
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return nil
		}
		if strings.Contains(s, "\n") {
			lines := strings.Split(s, "\n")
			out := make([]string, 0, len(lines))
			for _, l := range lines {
				if t := strings.TrimSpace(l); t != "" {
					out = append(out, html.UnescapeString(t))
				}
			}
			return out
		}
		return []string{html.UnescapeString(s)}
	case []any:
		out := make([]string, 0, len(x))
		for _, it := range x {
			switch v2 := it.(type) {
			case string:
				if s := strings.TrimSpace(v2); s != "" {
					out = append(out, html.UnescapeString(s))
				}
			case map[string]any:
				if s := asString(v2["name"]); s != "" {
					out = append(out, s)
				} else if s := asString(v2["text"]); s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	}
	return nil
}

func extractKeywords(v any) []string {
	switch x := v.(type) {
	case string:
		parts := strings.Split(x, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
		return out
	case []any:
		return extractStringList(x)
	}
	return nil
}

func extractRating(v any) AggregateRating {
	ar := AggregateRating{}
	m, ok := v.(map[string]any)
	if !ok {
		return ar
	}
	if s := asString(m["ratingValue"]); s != "" {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			ar.Value = f
		}
	}
	if s := asString(m["reviewCount"]); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			ar.Count = n
		}
	}
	if ar.Count == 0 {
		if s := asString(m["ratingCount"]); s != "" {
			if n, err := strconv.Atoi(s); err == nil {
				ar.Count = n
			}
		}
	}
	return ar
}

func extractNutrition(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for k, val := range m {
		if strings.HasPrefix(k, "@") {
			continue
		}
		if s := asString(val); s != "" {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// CleanInstructions normalizes the `recipeInstructions` field which may be:
//   - a plain string (split on newlines)
//   - a list of strings
//   - a list of HowToStep objects
//   - a list of HowToSection objects (with nested itemListElement)
func CleanInstructions(raw any) []string {
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case string:
		lines := strings.Split(v, "\n")
		out := make([]string, 0, len(lines))
		for _, l := range lines {
			if t := strings.TrimSpace(l); t != "" {
				out = append(out, html.UnescapeString(t))
			}
		}
		return out
	case []any:
		out := []string{}
		for _, item := range v {
			switch x := item.(type) {
			case string:
				if s := strings.TrimSpace(x); s != "" {
					out = append(out, html.UnescapeString(s))
				}
			case map[string]any:
				t, _ := x["@type"].(string)
				switch t {
				case "HowToSection":
					if name := asString(x["name"]); name != "" {
						out = append(out, "— "+name+" —")
					}
					if nested, ok := x["itemListElement"]; ok {
						out = append(out, CleanInstructions(nested)...)
					}
				default:
					if s := asString(x["text"]); s != "" {
						out = append(out, s)
					} else if s := asString(x["name"]); s != "" {
						out = append(out, s)
					}
				}
			}
		}
		return out
	}
	return nil
}

// ParseISO8601Duration converts an ISO 8601 duration string (e.g. "PT1H30M")
// into seconds. Returns 0 for empty or invalid input. Supports H, M, S fields;
// day-and-larger fields are ignored (recipes shouldn't have them).
func ParseISO8601Duration(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Accept trailing-only numeric input that skipped the PT prefix.
	if !strings.HasPrefix(s, "P") && !strings.HasPrefix(s, "p") {
		if n, err := strconv.Atoi(s); err == nil {
			return n * 60
		}
		return 0
	}
	upper := strings.ToUpper(s)
	// Strip P and possibly T.
	body := strings.TrimPrefix(upper, "P")
	// Remove day portion if present (e.g., "1DT2H"): we treat 1 day = 86400.
	var total int
	if idx := strings.Index(body, "T"); idx >= 0 {
		datePart := body[:idx]
		body = body[idx+1:]
		if datePart != "" {
			total += parseDurationPart(datePart, map[byte]int{'D': 86400, 'W': 604800})
		}
	}
	total += parseDurationPart(body, map[byte]int{'H': 3600, 'M': 60, 'S': 1})
	return total
}

func parseDurationPart(body string, units map[byte]int) int {
	total := 0
	num := ""
	for i := 0; i < len(body); i++ {
		c := body[i]
		if (c >= '0' && c <= '9') || c == '.' {
			num += string(c)
			continue
		}
		if mult, ok := units[c]; ok && num != "" {
			if f, err := strconv.ParseFloat(num, 64); err == nil {
				total += int(f * float64(mult))
			}
		}
		num = ""
	}
	return total
}

// Ensure fmt import used in err formatting paths is kept even during refactors.
var _ = fmt.Sprintf
