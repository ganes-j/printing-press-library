package food52

// CuratedTags is the slug→display-name table for the recipe tags Food52
// surfaces in its primary navigation. We curate this rather than scrape the
// nav at runtime so `tags list` works offline and is stable across nav
// rewrites. When Food52 adds or removes a tag we update this table.
//
// Tags are grouped by Kind so `tags list --kind cuisine` (or meal,
// preparation, etc.) can filter sensibly.
var CuratedTags = []TagInfo{
	// Meals
	{Slug: "breakfast", Title: "Breakfast", Kind: "meal"},
	{Slug: "brunch", Title: "Brunch", Kind: "meal"},
	{Slug: "lunch", Title: "Lunch", Kind: "meal"},
	{Slug: "dinner", Title: "Dinner", Kind: "meal"},
	{Slug: "appetizer", Title: "Appetizers", Kind: "meal"},
	{Slug: "dessert", Title: "Sweet Treats", Kind: "meal"},
	{Slug: "snack", Title: "Snacks", Kind: "meal"},
	// Ingredients
	{Slug: "chicken", Title: "Chicken", Kind: "ingredient"},
	{Slug: "steak", Title: "Steak", Kind: "ingredient"},
	{Slug: "salmon", Title: "Salmon", Kind: "ingredient"},
	{Slug: "pasta", Title: "Pasta", Kind: "ingredient"},
	{Slug: "potato", Title: "Potato", Kind: "ingredient"},
	{Slug: "rice", Title: "Rice", Kind: "ingredient"},
	{Slug: "egg", Title: "Eggs", Kind: "ingredient"},
	{Slug: "tofu", Title: "Tofu", Kind: "ingredient"},
	{Slug: "beans", Title: "Beans", Kind: "ingredient"},
	// Preparation / tools
	{Slug: "one-pot-wonders", Title: "One Pot", Kind: "preparation"},
	{Slug: "sheet-pan", Title: "Sheet Pan", Kind: "preparation"},
	{Slug: "ice-cream-frozen-desserts", Title: "No Bake / Frozen", Kind: "preparation"},
	{Slug: "grill-barbecue", Title: "Grill", Kind: "preparation"},
	{Slug: "bake", Title: "Bake", Kind: "preparation"},
	{Slug: "instant-pot", Title: "Instant Pot", Kind: "preparation"},
	{Slug: "slow-cooker", Title: "Slow Cooker", Kind: "preparation"},
	// Lifestyle / dietary
	{Slug: "vegetarian", Title: "Vegetarian", Kind: "lifestyle"},
	{Slug: "vegan", Title: "Vegan", Kind: "lifestyle"},
	{Slug: "gluten-free", Title: "Gluten-Free", Kind: "lifestyle"},
	{Slug: "dairy-free", Title: "Dairy-Free", Kind: "lifestyle"},
	{Slug: "booze-free-drinks", Title: "Alcohol-Free Drinks", Kind: "lifestyle"},
	// Quickness / convenience
	{Slug: "quick-and-easy", Title: "Quick and Easy", Kind: "convenience"},
	{Slug: "5-ingredients-or-fewer", Title: "5 Ingredients or Fewer", Kind: "convenience"},
	{Slug: "30-minutes-or-fewer", Title: "30 Minutes or Fewer", Kind: "convenience"},
	{Slug: "weeknight", Title: "Weeknight", Kind: "convenience"},
	// Course / category
	{Slug: "salad", Title: "Salad", Kind: "course"},
	{Slug: "soup", Title: "Soup", Kind: "course"},
	{Slug: "sandwich", Title: "Sandwich", Kind: "course"},
	{Slug: "cocktail", Title: "Cocktails", Kind: "course"},
	// Cuisines (a sample — Food52's cuisine taxonomy is large; ship the most-used)
	{Slug: "italian", Title: "Italian", Kind: "cuisine"},
	{Slug: "french", Title: "French", Kind: "cuisine"},
	{Slug: "mexican", Title: "Mexican", Kind: "cuisine"},
	{Slug: "chinese", Title: "Chinese", Kind: "cuisine"},
	{Slug: "japanese", Title: "Japanese", Kind: "cuisine"},
	{Slug: "korean", Title: "Korean", Kind: "cuisine"},
	{Slug: "thai", Title: "Thai", Kind: "cuisine"},
	{Slug: "indian", Title: "Indian", Kind: "cuisine"},
	{Slug: "mediterranean", Title: "Mediterranean", Kind: "cuisine"},
	{Slug: "middle-eastern", Title: "Middle Eastern", Kind: "cuisine"},
	{Slug: "american", Title: "American", Kind: "cuisine"},
}

// TagInfo is a single tag row exposed by `tags list`.
type TagInfo struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
}

// FilterTagsByKind returns the curated tags whose Kind matches `kind`. An
// empty kind returns all tags.
func FilterTagsByKind(kind string) []TagInfo {
	if kind == "" {
		out := make([]TagInfo, len(CuratedTags))
		copy(out, CuratedTags)
		return out
	}
	out := []TagInfo{}
	for _, t := range CuratedTags {
		if t.Kind == kind {
			out = append(out, t)
		}
	}
	return out
}

// AllTagKinds returns the distinct Kind values in CuratedTags, in stable
// presentation order.
func AllTagKinds() []string {
	return []string{"meal", "course", "ingredient", "cuisine", "lifestyle", "preparation", "convenience"}
}

// IsKnownTag returns true when slug appears in the curated table. The CLI
// uses this to short-circuit obviously-bad tag arguments before a network
// round trip.
func IsKnownTag(slug string) bool {
	for _, t := range CuratedTags {
		if t.Slug == slug {
			return true
		}
	}
	return false
}
