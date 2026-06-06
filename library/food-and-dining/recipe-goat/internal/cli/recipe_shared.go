// Shared helpers for the Phase-3 recipe subsystem commands. Centralizes the
// DB path resolution, recipe <-> stored conversion, and formatting used by
// `recipe`, `save`, `cookbook`, `goat`, `meal-plan`, `cook`, etc.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/enetx/surf"

	"github.com/mvanhorn/printing-press-library/library/food-and-dining/recipe-goat/internal/recipes"
	"github.com/mvanhorn/printing-press-library/library/food-and-dining/recipe-goat/internal/store"
)

// recipeDBPath returns the SQLite DB path (~/.config/recipe-goat-pp-cli/recipes.db).
// Using a dedicated file keeps the Phase-3 recipe store separate from the
// USDA data cache and makes it easy for users to back up just their cookbook.
func recipeDBPath() string {
	if v := os.Getenv("RECIPE_GOAT_DB"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "recipe-goat-pp-cli", "recipes.db")
}

func openRecipeStore() (*store.Store, error) {
	return store.Open(recipeDBPath())
}

// httpClientForSites returns an *http.Client tuned for site scraping.
// The global --rate-limit flag is ignored here on purpose: these are
// third-party sites we crawl, not the USDA API.
//
// Backed by enetx/surf with Chrome impersonation: TLS fingerprint and
// HTTP/2 frame sequencing match a real Chrome session, which bypasses
// Dotdash/Cloudflare/PerimeterX bot screens that 403/429 a plain Go
// stdlib http.Client. This is the upgrade that re-enabled AllRecipes
// and Food52 fan-out after the 2026-04-13 baseline removed them.
func httpClientForSites(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	c := surf.NewClient().Builder().Impersonate().Chrome().Timeout(timeout).Session().Build().Unwrap()
	hc := c.Std()
	hc.Timeout = timeout
	return hc
}

// recipeToStored converts the parsed schema.org Recipe into our store DTO.
// JSON fields are pre-marshaled as strings since SQLite stores them as TEXT.
func recipeToStored(r *recipes.Recipe) *store.StoredRecipe {
	out := &store.StoredRecipe{
		URL:         r.URL,
		Site:        r.Site,
		Title:       r.Name,
		Author:      r.Author,
		ImageURL:    r.Image,
		TotalTimeS:  r.TotalTime,
		PrepTimeS:   r.PrepTime,
		CookTimeS:   r.CookTime,
		Servings:    recipes.ParseYield(r.RecipeYield),
		Rating:      r.AggregateRating.Value,
		ReviewCount: r.AggregateRating.Count,
		Description: r.Description,
		Ingredients: r.RecipeIngredient,
		FetchedAt:   r.FetchedAt,
	}
	if b, _ := json.Marshal(r.RecipeIngredient); len(b) > 0 {
		out.IngredientsJSON = string(b)
	}
	if b, _ := json.Marshal(r.RecipeInstructions); len(b) > 0 {
		out.InstructionsJSON = string(b)
	}
	if len(r.Nutrition) > 0 {
		if b, _ := json.Marshal(r.Nutrition); len(b) > 0 {
			out.NutritionJSON = string(b)
		}
	}
	if len(r.Keywords) > 0 {
		if b, _ := json.Marshal(r.Keywords); len(b) > 0 {
			out.KeywordsJSON = string(b)
		}
	}
	if len(r.RecipeCategory) > 0 {
		if b, _ := json.Marshal(r.RecipeCategory); len(b) > 0 {
			out.CategoriesJSON = string(b)
		}
	}
	if len(r.RecipeCuisine) > 0 {
		if b, _ := json.Marshal(r.RecipeCuisine); len(b) > 0 {
			out.CuisinesJSON = string(b)
		}
	}
	return out
}

// storedToRecipe reverses recipeToStored for `recipe get` style output when we
// only have a stored copy.
func storedToRecipe(sr *store.StoredRecipe) *recipes.Recipe {
	r := &recipes.Recipe{
		URL:         sr.URL,
		Name:        sr.Title,
		Author:      sr.Author,
		Image:       sr.ImageURL,
		Description: sr.Description,
		RecipeYield: fmt.Sprintf("%d", sr.Servings),
		PrepTime:    sr.PrepTimeS,
		CookTime:    sr.CookTimeS,
		TotalTime:   sr.TotalTimeS,
		Site:        sr.Site,
		FetchedAt:   sr.FetchedAt,
		AggregateRating: recipes.AggregateRating{
			Value: sr.Rating,
			Count: sr.ReviewCount,
		},
	}
	if sr.IngredientsJSON != "" {
		_ = json.Unmarshal([]byte(sr.IngredientsJSON), &r.RecipeIngredient)
	}
	if sr.InstructionsJSON != "" {
		_ = json.Unmarshal([]byte(sr.InstructionsJSON), &r.RecipeInstructions)
	}
	if sr.NutritionJSON != "" {
		_ = json.Unmarshal([]byte(sr.NutritionJSON), &r.Nutrition)
	}
	if sr.KeywordsJSON != "" {
		_ = json.Unmarshal([]byte(sr.KeywordsJSON), &r.Keywords)
	}
	if sr.CategoriesJSON != "" {
		_ = json.Unmarshal([]byte(sr.CategoriesJSON), &r.RecipeCategory)
	}
	if sr.CuisinesJSON != "" {
		_ = json.Unmarshal([]byte(sr.CuisinesJSON), &r.RecipeCuisine)
	}
	return r
}

// parseDurationShorthand turns "30m", "2h", "7d", "30s" into time.Duration.
// Falls back to time.ParseDuration for standard inputs like "1h30m".
func parseDurationShorthand(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	// Handle day suffix which time.ParseDuration doesn't support.
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// formatDuration renders seconds as "1h 30m" / "45m" / "45s".
func formatDuration(seconds int) string {
	if seconds <= 0 {
		return "—"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	case m > 0:
		return fmt.Sprintf("%dm", m)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// printRecipeCard renders a human-friendly recipe card. When md is true, emits
// Markdown; otherwise emits a plain-text card.
func printRecipeCard(w interface {
	Write(p []byte) (int, error)
}, r *recipes.Recipe, md bool) {
	if md {
		fmt.Fprintf(w, "# %s\n\n", r.Name)
		if r.Author != "" {
			fmt.Fprintf(w, "_by %s_\n\n", r.Author)
		}
		if r.Site != "" {
			fmt.Fprintf(w, "Source: %s (%s)\n\n", r.Site, r.URL)
		}
		if r.AggregateRating.Value > 0 {
			fmt.Fprintf(w, "Rating: %.2f (%d reviews)\n\n", r.AggregateRating.Value, r.AggregateRating.Count)
		}
		fmt.Fprintf(w, "Total time: %s · Prep: %s · Cook: %s · Yield: %s\n\n",
			formatDuration(r.TotalTime), formatDuration(r.PrepTime), formatDuration(r.CookTime), defaultString(r.RecipeYield, "—"))
		if r.Description != "" {
			fmt.Fprintf(w, "%s\n\n", r.Description)
		}
		fmt.Fprintf(w, "## Ingredients\n\n")
		for _, ing := range r.RecipeIngredient {
			fmt.Fprintf(w, "- %s\n", ing)
		}
		fmt.Fprintf(w, "\n## Instructions\n\n")
		for i, step := range r.RecipeInstructions {
			fmt.Fprintf(w, "%d. %s\n", i+1, step)
		}
		if len(r.Nutrition) > 0 {
			fmt.Fprintf(w, "\n## Nutrition (per serving)\n\n")
			for k, v := range r.Nutrition {
				fmt.Fprintf(w, "- %s: %s\n", k, v)
			}
		}
		return
	}

	// Plain card.
	fmt.Fprintf(w, "%s\n", bold(r.Name))
	if r.Author != "" {
		fmt.Fprintf(w, "by %s\n", r.Author)
	}
	fmt.Fprintf(w, "%s · %s\n", r.Site, r.URL)
	if r.AggregateRating.Value > 0 {
		fmt.Fprintf(w, "Rating: %.2f (%d reviews)\n", r.AggregateRating.Value, r.AggregateRating.Count)
	}
	fmt.Fprintf(w, "Total: %s · Prep: %s · Cook: %s · Yield: %s\n",
		formatDuration(r.TotalTime), formatDuration(r.PrepTime), formatDuration(r.CookTime), defaultString(r.RecipeYield, "—"))
	if r.Description != "" {
		fmt.Fprintf(w, "\n%s\n", r.Description)
	}
	fmt.Fprintf(w, "\nINGREDIENTS\n")
	for _, ing := range r.RecipeIngredient {
		fmt.Fprintf(w, "  • %s\n", ing)
	}
	fmt.Fprintf(w, "\nINSTRUCTIONS\n")
	for i, step := range r.RecipeInstructions {
		fmt.Fprintf(w, "  %d. %s\n", i+1, step)
	}
	if len(r.Nutrition) > 0 {
		fmt.Fprintf(w, "\nNUTRITION (per serving)\n")
		for k, v := range r.Nutrition {
			fmt.Fprintf(w, "  %s: %s\n", k, v)
		}
	}
}

func defaultString(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// siteHostsFromCSV splits a comma-separated list, trimming whitespace, and
// returns the subset of recipes.Sites whose Hostname is in the list. An empty
// or "all" input returns the full site registry.
func siteHostsFromCSV(csv string) []recipes.Site {
	csv = strings.TrimSpace(csv)
	if csv == "" || csv == "all" {
		out := make([]recipes.Site, len(recipes.Sites))
		copy(out, recipes.Sites)
		return out
	}
	allow := map[string]bool{}
	for _, h := range strings.Split(csv, ",") {
		h = strings.TrimSpace(strings.ToLower(h))
		h = strings.TrimPrefix(h, "www.")
		if h == "" {
			continue
		}
		allow[h] = true
		// Also support bare name (e.g., "budgetbytes") → "budgetbytes.com".
		for _, s := range recipes.Sites {
			if strings.HasPrefix(s.Hostname, h+".") || strings.HasPrefix(s.Hostname, h) {
				allow[s.Hostname] = true
			}
		}
	}
	out := []recipes.Site{}
	for _, s := range recipes.Sites {
		if allow[s.Hostname] {
			out = append(out, s)
		}
	}
	return out
}

// withContext applies the root flag timeout to a background context.
func (f *rootFlags) withContext() (context.Context, context.CancelFunc) {
	t := f.timeout
	if t <= 0 {
		t = 30 * time.Second
	}
	return context.WithTimeout(context.Background(), t)
}
