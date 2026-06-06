package food52

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// RecipeSummary is the lightweight shape returned from tag-browse and search
// listings. It carries enough to render a card or pick a slug for a follow-up
// `recipes get` call without a second round trip.
type RecipeSummary struct {
	ID                  string   `json:"id"`
	Slug                string   `json:"slug"`
	Title               string   `json:"title"`
	URL                 string   `json:"url"`
	Description         string   `json:"description,omitempty"`
	FeaturedImageURL    string   `json:"featured_image_url,omitempty"`
	AverageRating       float64  `json:"average_rating,omitempty"`
	RatingCount         int      `json:"rating_count,omitempty"`
	TestKitchenApproved bool     `json:"test_kitchen_approved,omitempty"`
	Tags                []string `json:"tags,omitempty"`
	PublishedAt         string   `json:"published_at,omitempty"`
}

// Recipe is the structured detail shape returned from `recipes get`. It merges
// the SSR pageProps.recipe object with the page's Schema.org Recipe JSON-LD
// when the latter is present (richer ingredients/instructions/yield/timings).
type Recipe struct {
	ID                  string   `json:"id"`
	Slug                string   `json:"slug"`
	Title               string   `json:"title"`
	URL                 string   `json:"url"`
	Description         string   `json:"description,omitempty"`
	AuthorName          string   `json:"author_name,omitempty"`
	AuthorSlug          string   `json:"author_slug,omitempty"`
	FeaturedImageURL    string   `json:"featured_image_url,omitempty"`
	Yield               string   `json:"yield,omitempty"`
	PrepTime            string   `json:"prep_time,omitempty"`
	CookTime            string   `json:"cook_time,omitempty"`
	TotalTime           string   `json:"total_time,omitempty"`
	Category            []string `json:"category,omitempty"`
	Cuisine             []string `json:"cuisine,omitempty"`
	Keywords            []string `json:"keywords,omitempty"`
	Ingredients         []string `json:"ingredients"`
	Instructions        []string `json:"instructions"`
	KitchenNotes        string   `json:"kitchen_notes,omitempty"`
	AverageRating       float64  `json:"average_rating,omitempty"`
	RatingCount         int      `json:"rating_count,omitempty"`
	TestKitchenApproved bool     `json:"test_kitchen_approved,omitempty"`
	Tags                []string `json:"tags,omitempty"`
	PublishedAt         string   `json:"published_at,omitempty"`
}

// ExtractRecipesByTag pulls a page of RecipeSummary records out of an HTML
// page rendered for /recipes/<tag>. The data lives at
// __NEXT_DATA__.props.pageProps.recipesByTag.results.
func ExtractRecipesByTag(html []byte) ([]RecipeSummary, string, error) {
	nd, err := ExtractNextData(html)
	if err != nil {
		return nil, "", err
	}
	pp, err := PageProps(nd)
	if err != nil {
		return nil, "", err
	}
	tagInfo, _ := pp["tag"].(map[string]any)
	tagName := stringField(tagInfo, "title")
	rbt, ok := pp["recipesByTag"].(map[string]any)
	if !ok {
		return nil, tagName, fmt.Errorf("food52: pageProps.recipesByTag missing (likely an unknown tag)")
	}
	resultsRaw, _ := rbt["results"].([]any)
	out := make([]RecipeSummary, 0, len(resultsRaw))
	for _, r := range resultsRaw {
		obj, ok := r.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, summaryFromSSR(obj))
	}
	return out, tagName, nil
}

// ExtractRecipe pulls a Recipe from a /recipes/<slug> page. It prefers the
// SSR pageProps.recipe payload (which carries ratings, kitchen notes, author)
// and overlays Schema.org JSON-LD for ingredients, instructions, and timings
// where the SSR shape is incomplete.
func ExtractRecipe(html []byte, canonicalURL string) (*Recipe, error) {
	nd, err := ExtractNextData(html)
	if err != nil {
		return nil, err
	}
	pp, err := PageProps(nd)
	if err != nil {
		return nil, err
	}
	rec, ok := pp["recipe"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("food52: pageProps.recipe missing (likely a 404 or renamed slug)")
	}

	r := &Recipe{
		ID:                  stringField(rec, "_id"),
		Slug:                stringField(rec, "slug"),
		Title:               stringField(rec, "title"),
		URL:                 canonicalURL,
		Description:         flattenSanityBlocks(rec["description"]),
		AuthorName:          firstNonEmpty(stringField(rec, "authorName"), stringField(rec, "resolvedAuthorName")),
		AuthorSlug:          firstNonEmpty(stringField(rec, "resolvedAuthorSlug"), stringField(rec, "authorSlug")),
		FeaturedImageURL:    extractImageURL(rec["featuredImage"]),
		AverageRating:       floatField(rec, "averageRating"),
		RatingCount:         intField(rec, "ratingCount"),
		TestKitchenApproved: boolField(rec, "testKitchenApproved"),
		KitchenNotes:        flattenSanityBlocks(rec["kitchenNotes"]),
		PublishedAt:         stringField(rec, "publishedAt"),
		Tags:                tagNames(rec["tags"]),
	}

	// recipeDetails carries ingredients/instructions/yield/times in the SSR shape.
	if details, ok := rec["recipeDetails"].(map[string]any); ok {
		r.Ingredients = sanityIngredientLines(details["ingredients"])
		r.Instructions = sanityInstructionLines(details["instructions"])
		r.Yield = stringField(details, "servings")
		r.PrepTime = stringField(details, "prepTime")
		r.CookTime = stringField(details, "cookTime")
		r.TotalTime = stringField(details, "totalTime")
	}

	// JSON-LD overlay — most reliable source for cleaned ingredient strings,
	// numbered instructions, ISO-8601 durations, and recipeYield.
	if ld := extractSchemaRecipeJSONLD(html); ld != nil {
		if r.Yield == "" {
			r.Yield = ld.RecipeYield
		}
		if r.PrepTime == "" {
			r.PrepTime = ld.PrepTime
		}
		if r.CookTime == "" {
			r.CookTime = ld.CookTime
		}
		if r.TotalTime == "" {
			r.TotalTime = ld.TotalTime
		}
		if len(r.Ingredients) == 0 {
			r.Ingredients = ld.RecipeIngredient
		}
		// Food52 occasionally pre-renders JSON-LD recipeIngredient strings
		// with literal " undefined " tokens where the source CMS field was
		// unset; strip them so cooks don't see "4 undefined tablespoons".
		r.Ingredients = cleanIngredientStrings(r.Ingredients)
		if len(r.Instructions) == 0 {
			r.Instructions = ld.instructionStrings()
		}
		r.Category = stringSlice(ld.RecipeCategory)
		r.Cuisine = stringSlice(ld.RecipeCuisine)
		r.Keywords = splitKeywords(ld.Keywords)
	}

	return r, nil
}

// summaryFromSSR builds a RecipeSummary from a SSR result row.
func summaryFromSSR(obj map[string]any) RecipeSummary {
	slug := stringField(obj, "slug")
	url := ""
	if slug != "" {
		url = "https://food52.com/recipes/" + slug
	}
	return RecipeSummary{
		ID:                  stringField(obj, "_id"),
		Slug:                slug,
		Title:               stringField(obj, "title"),
		URL:                 url,
		Description:         flattenSanityBlocks(obj["description"]),
		FeaturedImageURL:    extractImageURL(obj["featuredImage"]),
		AverageRating:       floatField(obj, "averageRating"),
		RatingCount:         intField(obj, "ratingCount"),
		TestKitchenApproved: boolField(obj, "testKitchenApproved"),
		Tags:                tagNames(obj["tags"]),
		PublishedAt:         stringField(obj, "publishedAt"),
	}
}

// jsonldRecipe is the subset of the Schema.org Recipe shape Food52 emits.
type jsonldRecipe struct {
	Type             string          `json:"@type"`
	Name             string          `json:"name"`
	RecipeYield      string          `json:"recipeYield"`
	PrepTime         string          `json:"prepTime"`
	CookTime         string          `json:"cookTime"`
	TotalTime        string          `json:"totalTime"`
	RecipeIngredient []string        `json:"recipeIngredient"`
	RecipeInstr      json.RawMessage `json:"recipeInstructions"`
	RecipeCategory   json.RawMessage `json:"recipeCategory"`
	RecipeCuisine    json.RawMessage `json:"recipeCuisine"`
	Keywords         string          `json:"keywords"`
}

func (j *jsonldRecipe) instructionStrings() []string {
	if len(j.RecipeInstr) == 0 {
		return nil
	}
	// Try []HowToStep first
	var steps []map[string]any
	if err := json.Unmarshal(j.RecipeInstr, &steps); err == nil {
		out := make([]string, 0, len(steps))
		for _, s := range steps {
			if t, ok := s["text"].(string); ok && t != "" {
				out = append(out, strings.TrimSpace(t))
			} else if n, ok := s["name"].(string); ok && n != "" {
				out = append(out, strings.TrimSpace(n))
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	// Try []string
	var raw []string
	if err := json.Unmarshal(j.RecipeInstr, &raw); err == nil {
		return raw
	}
	// Try a single string
	var single string
	if err := json.Unmarshal(j.RecipeInstr, &single); err == nil {
		// Split on newlines as a courtesy
		lines := strings.Split(single, "\n")
		out := make([]string, 0, len(lines))
		for _, l := range lines {
			if t := strings.TrimSpace(l); t != "" {
				out = append(out, t)
			}
		}
		return out
	}
	return nil
}

var jsonldRe = regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`)

func extractSchemaRecipeJSONLD(html []byte) *jsonldRecipe {
	for _, m := range jsonldRe.FindAllSubmatch(html, -1) {
		raw := m[1]
		// May be a single object or an array
		var single map[string]json.RawMessage
		if err := json.Unmarshal(raw, &single); err == nil {
			if t, ok := single["@type"]; ok && jsonStringEq(t, "Recipe") {
				var r jsonldRecipe
				if err := json.Unmarshal(raw, &r); err == nil {
					return &r
				}
			}
			continue
		}
		var arr []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &arr); err == nil {
			for _, item := range arr {
				if t, ok := item["@type"]; ok && jsonStringEq(t, "Recipe") {
					full, _ := json.Marshal(item)
					var r jsonldRecipe
					if err := json.Unmarshal(full, &r); err == nil {
						return &r
					}
				}
			}
		}
	}
	return nil
}
