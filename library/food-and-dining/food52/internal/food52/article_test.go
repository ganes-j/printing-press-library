package food52

import (
	"strings"
	"testing"
)

func TestExtractArticlesByVertical_FoodFixture(t *testing.T) {
	html := loadFixture(t, "food-vertical.html")
	results, err := ExtractArticlesByVertical(html)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one article, got 0")
	}
	first := results[0]
	if first.Title == "" {
		t.Error("first article Title is empty")
	}
	if first.Slug == "" {
		t.Error("first article Slug is empty")
	}
	if first.URL == "" || !strings.HasPrefix(first.URL, "https://food52.com/story/") {
		t.Errorf("first article URL malformed: %q", first.URL)
	}
}

func TestExtractArticle_StoryFixture(t *testing.T) {
	html := loadFixture(t, "story-detail.html")
	canonicalURL := "https://food52.com/story/best-mothers-day-gift-ideas"
	a, err := ExtractArticle(html, canonicalURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a.Title == "" {
		t.Error("Article.Title is empty")
	}
	if a.URL != canonicalURL {
		t.Errorf("URL: got %q, want %q", a.URL, canonicalURL)
	}
	if a.Body == "" {
		t.Error("Article.Body is empty (Sanity content should flatten to text)")
	}
}
