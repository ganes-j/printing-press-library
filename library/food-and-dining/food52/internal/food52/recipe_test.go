package food52

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("loading fixture %s: %v", name, err)
	}
	return data
}

func TestExtractRecipesByTag_ChickenFixture(t *testing.T) {
	html := loadFixture(t, "recipes-chicken.html")
	results, tag, err := ExtractRecipesByTag(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag != "Chicken" {
		t.Errorf("tag name: got %q, want %q", tag, "Chicken")
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result, got 0")
	}
	// Sanity check the first result has the fields we project.
	first := results[0]
	if first.Title == "" {
		t.Error("first result Title is empty")
	}
	if first.Slug == "" {
		t.Error("first result Slug is empty")
	}
	if first.URL == "" || !strings.HasPrefix(first.URL, "https://food52.com/recipes/") {
		t.Errorf("first result URL malformed: %q", first.URL)
	}
	if first.ID == "" {
		t.Error("first result ID is empty")
	}
}

func TestExtractRecipe_DetailFixture(t *testing.T) {
	html := loadFixture(t, "recipes-detail.html")
	canonicalURL := "https://food52.com/recipes/mom-s-japanese-curry-chicken-with-radish-and-cauliflower"
	r, err := ExtractRecipe(html, canonicalURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Title == "" {
		t.Fatal("Recipe.Title is empty")
	}
	if !strings.Contains(strings.ToLower(r.Title), "curry") {
		t.Errorf("expected title to mention curry, got %q", r.Title)
	}
	if r.URL != canonicalURL {
		t.Errorf("URL: got %q, want %q", r.URL, canonicalURL)
	}
	if r.ID == "" {
		t.Error("Recipe.ID is empty")
	}
	if len(r.Ingredients) < 5 {
		t.Errorf("expected at least 5 ingredients, got %d", len(r.Ingredients))
	}
	if len(r.Instructions) == 0 {
		t.Error("expected at least one instruction step")
	}
	if r.Yield == "" {
		t.Error("Recipe.Yield is empty (recipeYield should be present in JSON-LD)")
	}
}

func TestExtractNextData_NoMatch(t *testing.T) {
	html := []byte("<html><body>nothing here</body></html>")
	_, err := ExtractNextData(html)
	if err != ErrNoNextData {
		t.Errorf("expected ErrNoNextData, got %v", err)
	}
}

func TestLooksLikeChallenge_VercelHTML(t *testing.T) {
	html := []byte(`<title>Vercel Security Checkpoint</title>`)
	if !LooksLikeChallenge(html) {
		t.Error("expected LooksLikeChallenge to detect Vercel checkpoint title")
	}
	if LooksLikeChallenge([]byte("normal page")) {
		t.Error("expected LooksLikeChallenge to NOT flag normal page")
	}
}

func TestExtractRecipesByTag_EmptyHTML(t *testing.T) {
	_, _, err := ExtractRecipesByTag([]byte("<html></html>"))
	if err == nil {
		t.Error("expected error on empty HTML")
	}
}
