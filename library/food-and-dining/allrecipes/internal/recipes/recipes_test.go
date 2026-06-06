package recipes

import (
	"strings"
	"testing"
)

func TestParseISO8601Duration(t *testing.T) {
	cases := []struct {
		in       string
		expected int
	}{
		{"PT30M", 1800},
		{"PT1H", 3600},
		{"PT1H30M", 5400},
		{"PT2H45M30S", 9930},
		{"PT0M", 0},
		{"", 0},
		{"30", 1800}, // bare-int fallback (treated as minutes)
		{"PT15S", 15},
	}
	for _, c := range cases {
		got := ParseISO8601Duration(c.in)
		if got != c.expected {
			t.Errorf("ParseISO8601Duration(%q) = %d; want %d", c.in, got, c.expected)
		}
	}
}

func TestParseJSONLD_Basic(t *testing.T) {
	htmlBody := []byte(`<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"Recipe","name":"Test Recipe","recipeIngredient":["1 cup flour","2 eggs"],"recipeInstructions":[{"@type":"HowToStep","text":"Mix"},{"@type":"HowToStep","text":"Bake"}],"totalTime":"PT45M","aggregateRating":{"@type":"AggregateRating","ratingValue":"4.5","reviewCount":"100"}}
</script></head><body></body></html>`)

	r, err := ParseJSONLD(htmlBody, "https://www.allrecipes.com/recipe/9999/test/")
	if err != nil {
		t.Fatalf("ParseJSONLD failed: %v", err)
	}
	if r.Name != "Test Recipe" {
		t.Errorf("Name = %q; want %q", r.Name, "Test Recipe")
	}
	if len(r.RecipeIngredient) != 2 || r.RecipeIngredient[0] != "1 cup flour" {
		t.Errorf("RecipeIngredient = %v", r.RecipeIngredient)
	}
	if len(r.RecipeInstructions) != 2 || r.RecipeInstructions[0] != "Mix" {
		t.Errorf("RecipeInstructions = %v", r.RecipeInstructions)
	}
	if r.TotalTime != 2700 {
		t.Errorf("TotalTime = %d; want 2700", r.TotalTime)
	}
	if r.AggregateRating.Value != 4.5 || r.AggregateRating.Count != 100 {
		t.Errorf("AggregateRating = %+v", r.AggregateRating)
	}
	if r.Site != "allrecipes.com" {
		t.Errorf("Site = %q; want %q", r.Site, "allrecipes.com")
	}
}

func TestParseJSONLD_Graph(t *testing.T) {
	// Some sites wrap Recipe in @graph alongside other schema types.
	htmlBody := []byte(`<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org","@graph":[{"@type":"WebPage","name":"page"},{"@type":"Recipe","name":"Graph Recipe","recipeIngredient":["a","b"]}]}
</script></head></html>`)

	r, err := ParseJSONLD(htmlBody, "https://www.allrecipes.com/recipe/1/x/")
	if err != nil {
		t.Fatalf("ParseJSONLD failed: %v", err)
	}
	if r.Name != "Graph Recipe" {
		t.Errorf("Name = %q; want Graph Recipe", r.Name)
	}
}

func TestParseJSONLD_Missing(t *testing.T) {
	htmlBody := []byte(`<html><body>No JSON-LD here</body></html>`)
	_, err := ParseJSONLD(htmlBody, "x")
	if err != ErrNoJSONLD {
		t.Errorf("expected ErrNoJSONLD; got %v", err)
	}
}

func TestParseSearchResults(t *testing.T) {
	htmlBody := []byte(`<html><body>
<a href="https://www.allrecipes.com/recipe/9599/quick-and-easy-brownies/" class="card">
  <img src="https://example.com/brownies.jpg" alt="Brownies">
  <span class="card__title-text">Quick and Easy Brownies</span>
  <div class="mntl-recipe-card-meta__rating">4.7 (2,040)</div>
</a>
<a href="https://www.allrecipes.com/recipe/68436/vegan-brownies/" class="card">
  <img src="https://example.com/vegan.jpg" alt="Vegan">
  <span class="card__title-text">Vegan Brownies</span>
  <span>120 Ratings</span>
</a>
</body></html>`)

	results := ParseSearchResults(htmlBody, 10)
	if len(results) != 2 {
		t.Fatalf("len(results) = %d; want 2", len(results))
	}
	if results[0].Title != "Quick and Easy Brownies" {
		t.Errorf("results[0].Title = %q", results[0].Title)
	}
	if results[0].RecipeID != "9599" || results[0].Slug != "quick-and-easy-brownies" {
		t.Errorf("results[0] id/slug = %q/%q", results[0].RecipeID, results[0].Slug)
	}
	if results[0].Rating != 4.7 || results[0].ReviewCount != 2040 {
		t.Errorf("results[0] rating/count = %v/%d", results[0].Rating, results[0].ReviewCount)
	}
	if results[1].ReviewCount != 120 {
		t.Errorf("results[1].ReviewCount = %d; want 120", results[1].ReviewCount)
	}
}

func TestParseSearchResults_Dedup(t *testing.T) {
	htmlBody := []byte(`<a href="https://www.allrecipes.com/recipe/100/x/?utm=1"><span class="card__title-text">A</span></a>
<a href="https://www.allrecipes.com/recipe/100/x/?utm=2"><span class="card__title-text">A</span></a>`)
	results := ParseSearchResults(htmlBody, 10)
	if len(results) != 1 {
		t.Errorf("expected 1 deduped result, got %d", len(results))
	}
}

func TestCanonicalRecipeURL(t *testing.T) {
	cases := []struct {
		id, slug, want string
	}{
		{"9599", "quick-and-easy-brownies", "https://www.allrecipes.com/recipe/9599/quick-and-easy-brownies/"},
		{"100", "", "https://www.allrecipes.com/recipe/100/"},
		{"", "any", ""},
	}
	for _, c := range cases {
		got := CanonicalRecipeURL(c.id, c.slug)
		if got != c.want {
			t.Errorf("CanonicalRecipeURL(%q,%q) = %q; want %q", c.id, c.slug, got, c.want)
		}
	}
}

func TestResolveRecipeURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://www.allrecipes.com/recipe/100/x/", "https://www.allrecipes.com/recipe/100/x/"},
		{"100", "https://www.allrecipes.com/recipe/100/"},
		{"100/test-slug", "https://www.allrecipes.com/recipe/100/test-slug/"},
		{"", ""},
	}
	for _, c := range cases {
		got := ResolveRecipeURL(c.in)
		if got != c.want {
			t.Errorf("ResolveRecipeURL(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestParseURL(t *testing.T) {
	id, slug := ParseURL("https://www.allrecipes.com/recipe/9599/quick-and-easy-brownies/")
	if id != "9599" || slug != "quick-and-easy-brownies" {
		t.Errorf("ParseURL = %q/%q; want 9599/quick-and-easy-brownies", id, slug)
	}
	id, slug = ParseURL("https://example.com/foo")
	if id != "" || slug != "" {
		t.Errorf("ParseURL on non-recipe URL should return empty; got %q/%q", id, slug)
	}
}

func TestParseIngredient(t *testing.T) {
	cases := []struct {
		in       string
		wantQty  float64
		wantUnit string
		wantName string
	}{
		{"2 cups white sugar", 2, "cups", "white sugar"},
		{"1.5 cups all-purpose flour", 1.5, "cups", "all-purpose flour"},
		{"4 eggs", 4, "", "eggs"},
		{"0.5 teaspoon vanilla extract", 0.5, "teaspoon", "vanilla extract"},
		{"1 1/2 cups water", 1.5, "cups", "water"},
		{"3/4 cup butter, melted", 0.75, "cup", "butter, melted"},
		{"baking spray", 0, "", "baking spray"},
		{"½ cup honey", 0.5, "cup", "honey"},
	}
	for _, c := range cases {
		got := ParseIngredient(c.in)
		if got.Quantity != c.wantQty || got.Unit != c.wantUnit || got.Name != c.wantName {
			t.Errorf("ParseIngredient(%q) = {qty=%v unit=%q name=%q}; want {qty=%v unit=%q name=%q}",
				c.in, got.Quantity, got.Unit, got.Name, c.wantQty, c.wantUnit, c.wantName)
		}
	}
}

func TestScaleIngredients(t *testing.T) {
	in := []ParsedIngredient{
		{Raw: "2 cups flour", Quantity: 2, Unit: "cups", Name: "flour"},
		{Raw: "1 teaspoon salt", Quantity: 1, Unit: "teaspoon", Name: "salt"},
		{Raw: "salt to taste", Name: "salt to taste"},
	}
	out := ScaleIngredients(in, 2)
	if out[0].Quantity != 4 {
		t.Errorf("scaled flour quantity = %v; want 4", out[0].Quantity)
	}
	if !strings.Contains(out[0].Raw, "4") {
		t.Errorf("scaled flour Raw = %q; expected '4'", out[0].Raw)
	}
	if out[2].Quantity != 0 {
		t.Errorf("ingredient without quantity should stay 0; got %v", out[2].Quantity)
	}
}

func TestAggregateGrocery(t *testing.T) {
	r1 := []ParsedIngredient{
		{Raw: "1 cup flour", Quantity: 1, Unit: "cup", Name: "flour"},
		{Raw: "2 eggs", Quantity: 2, Name: "eggs"},
	}
	r2 := []ParsedIngredient{
		{Raw: "2 cups flour", Quantity: 2, Unit: "cup", Name: "flour"},
		{Raw: "3 eggs", Quantity: 3, Name: "eggs"},
	}
	agg := AggregateGrocery([][]ParsedIngredient{r1, r2})
	if len(agg) != 2 {
		t.Fatalf("expected 2 aggregated entries, got %d", len(agg))
	}
	// flour should be summed to 3
	flour := agg[0]
	if flour.Name != "flour" || flour.Quantity != 3 {
		t.Errorf("flour aggregate = %+v; want qty=3", flour)
	}
	// eggs should be summed to 5
	eggs := agg[1]
	if eggs.Name != "eggs" || eggs.Quantity != 5 {
		t.Errorf("eggs aggregate = %+v; want qty=5", eggs)
	}
}

func TestBayesianRating(t *testing.T) {
	cases := []struct {
		rating       float64
		count        int
		priorMean    float64
		c            int
		wantApprox   float64
		toleranceAbs float64
	}{
		// Pure prior case (rating=0)
		{0, 0, 4.0, 200, 4.0, 0.001},
		// Many reviews — close to actual rating
		{4.7, 2000, 4.0, 200, 4.636, 0.01},
		// Few reviews — pulled toward prior
		{5.0, 1, 4.0, 200, 4.005, 0.01},
		// Same as actual when c=0
		{4.5, 100, 4.0, 0, 4.5, 0.001},
	}
	for _, c := range cases {
		got := BayesianRating(c.rating, c.count, c.priorMean, c.c)
		diff := got - c.wantApprox
		if diff < -c.toleranceAbs || diff > c.toleranceAbs {
			t.Errorf("BayesianRating(%v, %d, %v, %d) = %v; want ~%v", c.rating, c.count, c.priorMean, c.c, got, c.wantApprox)
		}
	}
}

func TestRank_OutlierShouldNotWin(t *testing.T) {
	// 5-star/1-review outlier should NOT outrank a 4.7/2000 proven recipe.
	results := []SearchResult{
		{URL: "a", Title: "Outlier", Rating: 5.0, ReviewCount: 1},
		{URL: "b", Title: "Proven", Rating: 4.7, ReviewCount: 2000},
	}
	ranked := Rank(results, 4.0, 200)
	if ranked[0].URL != "b" {
		t.Errorf("expected proven 4.7/2000 to outrank 5.0/1 outlier; got order: %s, %s", ranked[0].URL, ranked[1].URL)
	}
}

func TestFormatTime(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, ""},
		{60, "1m"},
		{1800, "30m"},
		{3600, "1h"},
		{5400, "1h 30m"},
		{9930, "2h 45m"},
	}
	for _, c := range cases {
		got := FormatTime(c.in)
		if got != c.want {
			t.Errorf("FormatTime(%d) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestCleanInstructions(t *testing.T) {
	// HowToStep array
	stepsStruct := []any{
		map[string]any{"@type": "HowToStep", "text": "Step 1"},
		map[string]any{"@type": "HowToStep", "text": "Step 2"},
	}
	out := CleanInstructions(stepsStruct)
	if len(out) != 2 || out[0] != "Step 1" {
		t.Errorf("CleanInstructions HowToStep = %v", out)
	}
	// Plain string
	out2 := CleanInstructions("Step A\nStep B\n")
	if len(out2) != 2 {
		t.Errorf("CleanInstructions plain string = %v", out2)
	}
	// HowToSection with nested itemListElement
	section := []any{
		map[string]any{
			"@type": "HowToSection",
			"name":  "Make the brownies",
			"itemListElement": []any{
				map[string]any{"@type": "HowToStep", "text": "Mix"},
			},
		},
	}
	out3 := CleanInstructions(section)
	if len(out3) != 2 || !strings.Contains(out3[0], "Make the brownies") || out3[1] != "Mix" {
		t.Errorf("CleanInstructions HowToSection = %v", out3)
	}
}
