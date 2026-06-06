package food52

import (
	"fmt"
)

// ArticleSummary is the listing shape returned from /<vertical> and
// /<vertical>/<sub> pages.
type ArticleSummary struct {
	ID               string   `json:"id"`
	Slug             string   `json:"slug"`
	Title            string   `json:"title"`
	URL              string   `json:"url"`
	Dek              string   `json:"dek,omitempty"`
	AuthorName       string   `json:"author_name,omitempty"`
	FeaturedImageURL string   `json:"featured_image_url,omitempty"`
	PublishedAt      string   `json:"published_at,omitempty"`
	Vertical         string   `json:"vertical,omitempty"`
	SubVertical      string   `json:"sub_vertical,omitempty"`
	Tags             []string `json:"tags,omitempty"`
}

// Article is the detail shape returned from /story/<slug>.
type Article struct {
	ID               string   `json:"id"`
	Slug             string   `json:"slug"`
	Title            string   `json:"title"`
	URL              string   `json:"url"`
	Dek              string   `json:"dek,omitempty"`
	AuthorName       string   `json:"author_name,omitempty"`
	AuthorSlug       string   `json:"author_slug,omitempty"`
	FeaturedImageURL string   `json:"featured_image_url,omitempty"`
	PublishedAt      string   `json:"published_at,omitempty"`
	Vertical         string   `json:"vertical,omitempty"`
	SubVertical      string   `json:"sub_vertical,omitempty"`
	Tags             []string `json:"tags,omitempty"`
	Body             string   `json:"body"`
	RelatedRecipes   []string `json:"related_recipes,omitempty"`
}

// ExtractArticlesByVertical pulls a page of ArticleSummary from /<vertical>
// or /<vertical>/<sub> SSR HTML.
func ExtractArticlesByVertical(html []byte) ([]ArticleSummary, error) {
	nd, err := ExtractNextData(html)
	if err != nil {
		return nil, err
	}
	pp, err := PageProps(nd)
	if err != nil {
		return nil, err
	}
	bp, ok := pp["blogPosts"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("food52: pageProps.blogPosts missing")
	}
	resultsRaw, _ := bp["results"].([]any)
	out := make([]ArticleSummary, 0, len(resultsRaw))
	for _, r := range resultsRaw {
		obj, ok := r.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, articleSummaryFromSSR(obj))
	}
	return out, nil
}

// ExtractArticle pulls a single Article from /story/<slug> SSR HTML.
func ExtractArticle(html []byte, canonicalURL string) (*Article, error) {
	nd, err := ExtractNextData(html)
	if err != nil {
		return nil, err
	}
	pp, err := PageProps(nd)
	if err != nil {
		return nil, err
	}
	bp, ok := pp["blogPost"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("food52: pageProps.blogPost missing (likely a 404 or renamed slug)")
	}

	a := &Article{
		ID:               stringField(bp, "_id"),
		Slug:             stringField(bp, "slug"),
		Title:            stringField(bp, "title"),
		URL:              canonicalURL,
		Dek:              stringField(bp, "dek"),
		AuthorName:       firstNonEmpty(stringField(bp, "authorName"), stringField(bp, "resolvedAuthorName")),
		AuthorSlug:       firstNonEmpty(stringField(bp, "resolvedAuthorSlug"), stringField(bp, "authorSlug")),
		FeaturedImageURL: extractImageURL(bp["featuredImage"]),
		PublishedAt:      stringField(bp, "publishedAt"),
		SubVertical:      stringField(bp, "subVertical"),
		Tags:             tagNames(bp["tags"]),
		Body:             flattenSanityBlocks(bp["content"]),
		RelatedRecipes:   relatedRecipeSlugs(bp["relatedReading"]),
	}
	if blog, ok := bp["blog"].(map[string]any); ok {
		a.Vertical = stringField(blog, "slug")
	}
	return a, nil
}

func articleSummaryFromSSR(obj map[string]any) ArticleSummary {
	slug := stringField(obj, "slug")
	url := ""
	if slug != "" {
		url = "https://food52.com/story/" + slug
	}
	a := ArticleSummary{
		ID:               stringField(obj, "_id"),
		Slug:             slug,
		Title:            stringField(obj, "title"),
		URL:              url,
		Dek:              stringField(obj, "dek"),
		AuthorName:       firstNonEmpty(stringField(obj, "authorName"), stringField(obj, "resolvedAuthorName")),
		FeaturedImageURL: extractImageURL(obj["featuredImage"]),
		PublishedAt:      stringField(obj, "publishedAt"),
		SubVertical:      stringField(obj, "subVertical"),
		Tags:             tagNames(obj["tags"]),
	}
	if blog, ok := obj["blog"].(map[string]any); ok {
		a.Vertical = stringField(blog, "slug")
	}
	return a
}

// relatedRecipeSlugs walks a relatedReading array and pulls out slugs that
// look like recipe references (anything pointing at /recipes/<slug>).
func relatedRecipeSlugs(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []string{}
	seen := map[string]bool{}
	for _, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// Sanity sometimes nests under target/document/etc.; collect any string
		// field that looks like a recipe slug or full recipe URL.
		walkForRecipeSlug(obj, &out, seen)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func walkForRecipeSlug(v any, out *[]string, seen map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		typeStr := stringField(t, "_type")
		if typeStr == "recipe" || typeStr == "recipeReference" {
			if s := stringField(t, "slug"); s != "" && !seen[s] {
				*out = append(*out, s)
				seen[s] = true
			}
		}
		for _, child := range t {
			walkForRecipeSlug(child, out, seen)
		}
	case []any:
		for _, child := range t {
			walkForRecipeSlug(child, out, seen)
		}
	case string:
		// Detect /recipes/<slug> URLs
		if i := indexFold(t, "/recipes/"); i >= 0 {
			rest := t[i+len("/recipes/"):]
			if end := indexAny(rest, "/?#"); end >= 0 {
				rest = rest[:end]
			}
			if rest != "" && !seen[rest] {
				*out = append(*out, rest)
				seen[rest] = true
			}
		}
	}
}
