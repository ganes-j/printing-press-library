package recipes

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/food-and-dining/allrecipes/internal/client"
)

// Client is the subset of *client.Client we need. Defined as an interface so
// commands and tests can inject a fake without dragging in the whole stack.
type Client interface {
	Get(path string, params map[string]string) (json.RawMessage, error)
}

var _ Client = (*client.Client)(nil)

// FetchRecipe fetches a recipe page by URL and parses the JSON-LD into Recipe.
// Returns ErrNoJSONLD if the page has no Recipe schema (rare; usually implies
// the URL is not a recipe page or the response is a Cloudflare interstitial).
func FetchRecipe(c Client, recipeURL string) (*Recipe, error) {
	if recipeURL == "" {
		return nil, fmt.Errorf("FetchRecipe: empty url")
	}
	path := strings.TrimPrefix(recipeURL, "https://www.allrecipes.com")
	path = strings.TrimPrefix(path, "https://allrecipes.com")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	body, err := c.Get(path, nil)
	if err != nil {
		return nil, err
	}
	r, err := ParseJSONLD(body, recipeURL)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// FetchSearch fetches an Allrecipes search results page and parses it into
// SearchResult records. `query` is the user's search term; `page` is 1-indexed
// (0 or 1 both mean the first page).
func FetchSearch(c Client, query string, page, limit int) ([]SearchResult, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("FetchSearch: empty query")
	}
	params := map[string]string{"q": q}
	if page > 1 {
		params["page"] = fmt.Sprintf("%d", page)
	}
	body, err := c.Get("/search", params)
	if err != nil {
		return nil, err
	}
	return ParseSearchResults(body, limit), nil
}

// FetchCategoryHTML fetches an Allrecipes category page (path-only, e.g.
// "/recipes/79/desserts/") and parses its recipe links.
func FetchCategoryHTML(c Client, path string, limit int) ([]SearchResult, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	body, err := c.Get(path, nil)
	if err != nil {
		return nil, err
	}
	return ParseSearchResults(body, limit), nil
}

// FetchHTML returns the raw HTML body for any path on allrecipes.com. Used by
// commands that scrape non-recipe pages (article, gallery, cook profile).
func FetchHTML(c Client, path string) ([]byte, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	body, err := c.Get(path, nil)
	if err != nil {
		return nil, err
	}
	return body, nil
}
