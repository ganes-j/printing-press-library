package recipes

import (
	"net/url"
	"regexp"
	"strings"
)

// Site describes a recipe source we search against.
type Site struct {
	Name      string
	Hostname  string
	Tier      int
	SearchURL string  // template with {q}
	Trust     float64 // 0..1

	// RecipeURLPattern, if non-nil, is matched against the *path portion* of
	// candidate URLs when deciding whether a link looks like a recipe permalink.
	// Populated in init() below — sourced from each site's real URL structure.
	RecipeURLPattern *regexp.Regexp
}

// Sites is the built-in registry of recipe sources.
//
// Curation principle: the list optimizes for sites that reliably return
// content to a real-browser HTTP client, not for sites with the highest
// traffic. Selection is based on (a) Recipe JSON-LD presence on permalink
// pages, (b) static-HTML search results that surface actual recipe
// permalinks (not just category links — Food52's search is JS-rendered
// and returns 0 candidates here even though `recipe get` works fine).
//
// History: Food52, Food Network, AllRecipes, Simply Recipes, EatingWell
// and Serious Eats were tiered as "blocked" or "fragile" in April 2026
// because plain Go HTTP got 403/429/EOF from Dotdash/Cloudflare/PerimeterX
// bot screens. As of 2026-04-26 the CLI uses enetx/surf with Chrome
// impersonation (TLS fingerprint + HTTP/2 frame ordering). Re-probes
// against the live sites confirmed:
//   - AllRecipes: 200 + Recipe JSON-LD; search returns real permalinks
//   - Food52: 200 + Recipe JSON-LD on `recipe get`; search HTML lacks
//     permalinks (JS-rendered) so search returns 0 — kept in Sites
//     because `recipe get`/`save` work for known URLs
//   - Food Network: search returns 18+ recipe permalinks
//   - Simply Recipes: search returns recipe permalinks → real Recipe page
//   - EatingWell: same
//   - Serious Eats: same — promoted from Tier 3 to Tier 1
//
// The Tier label is now a content-trust label only; it has no relationship
// to reachability since Surf clears every site in the registry.
var Sites = []Site{
	// Tier 1 — independent, high-authority, reliably reachable.
	{Name: "King Arthur Baking", Hostname: "kingarthurbaking.com", Tier: 1, SearchURL: "https://www.kingarthurbaking.com/search?f%5B0%5D=content_type%3Arecipe&q={q}", Trust: 0.9},
	{Name: "Budget Bytes", Hostname: "budgetbytes.com", Tier: 1, SearchURL: "https://www.budgetbytes.com/?s={q}", Trust: 0.9},
	{Name: "Smitten Kitchen", Hostname: "smittenkitchen.com", Tier: 1, SearchURL: "https://smittenkitchen.com/?s={q}", Trust: 0.9},
	{Name: "BBC Good Food", Hostname: "bbcgoodfood.com", Tier: 1, SearchURL: "https://www.bbcgoodfood.com/search?q={q}", Trust: 0.9},
	{Name: "BBC Food", Hostname: "bbc.co.uk", Tier: 1, SearchURL: "https://www.bbc.co.uk/food/search?q={q}", Trust: 0.9},
	{Name: "Minimalist Baker", Hostname: "minimalistbaker.com", Tier: 1, SearchURL: "https://minimalistbaker.com/?s={q}", Trust: 0.9},
	{Name: "Skinnytaste", Hostname: "skinnytaste.com", Tier: 1, SearchURL: "https://www.skinnytaste.com/?s={q}", Trust: 0.9},
	{Name: "The Kitchn", Hostname: "thekitchn.com", Tier: 1, SearchURL: "https://www.thekitchn.com/search?q={q}", Trust: 0.9},
	{Name: "RecipeTin Eats", Hostname: "recipetineats.com", Tier: 1, SearchURL: "https://www.recipetineats.com/?s={q}", Trust: 0.9},
	{Name: "The Woks of Life", Hostname: "thewoksoflife.com", Tier: 1, SearchURL: "https://thewoksoflife.com/?s={q}", Trust: 0.9},
	{Name: "Just One Cookbook", Hostname: "justonecookbook.com", Tier: 1, SearchURL: "https://www.justonecookbook.com/?s={q}", Trust: 0.9},
	{Name: "The Cozy Cook", Hostname: "thecozycook.com", Tier: 1, SearchURL: "https://thecozycook.com/?s={q}", Trust: 0.9},
	{Name: "The Mediterranean Dish", Hostname: "themediterraneandish.com", Tier: 1, SearchURL: "https://www.themediterraneandish.com/?s={q}", Trust: 0.9},
	{Name: "Kitchen Sanctuary", Hostname: "kitchensanctuary.com", Tier: 1, SearchURL: "https://www.kitchensanctuary.com/?s={q}", Trust: 0.9},
	{Name: "Grandbaby Cakes", Hostname: "grandbaby-cakes.com", Tier: 1, SearchURL: "https://grandbaby-cakes.com/?s={q}", Trust: 0.9},
	{Name: "My Korean Kitchen", Hostname: "mykoreankitchen.com", Tier: 1, SearchURL: "https://mykoreankitchen.com/?s={q}", Trust: 0.9},
	{Name: "Olivia's Cuisine", Hostname: "oliviascuisine.com", Tier: 1, SearchURL: "https://www.oliviascuisine.com/?s={q}", Trust: 0.9},
	{Name: "Feed the Pudge", Hostname: "feedthepudge.com", Tier: 1, SearchURL: "https://feedthepudge.com/?s={q}", Trust: 0.9},
	{Name: "Preppy Kitchen", Hostname: "preppykitchen.com", Tier: 1, SearchURL: "https://preppykitchen.com/?s={q}", Trust: 0.9},
	{Name: "Sally's Baking Addiction", Hostname: "sallysbakingaddiction.com", Tier: 1, SearchURL: "https://sallysbakingaddiction.com/?s={q}", Trust: 0.9},
	{Name: "Broma Bakery", Hostname: "bromabakery.com", Tier: 1, SearchURL: "https://bromabakery.com/?s={q}", Trust: 0.9},
	{Name: "Bigger Bolder Baking", Hostname: "biggerbolderbaking.com", Tier: 1, SearchURL: "https://www.biggerbolderbaking.com/?s={q}", Trust: 0.9},
	{Name: "The Cafe Sucre Farine", Hostname: "thecafesucrefarine.com", Tier: 1, SearchURL: "https://thecafesucrefarine.com/?s={q}", Trust: 0.9},
	{Name: "China Sichuan Food", Hostname: "chinasichuanfood.com", Tier: 1, SearchURL: "https://www.chinasichuanfood.com/?s={q}", Trust: 0.9},
	{Name: "Red House Spice", Hostname: "redhousespice.com", Tier: 1, SearchURL: "https://redhousespice.com/?s={q}", Trust: 0.9},
	{Name: "My Heart Beets", Hostname: "myheartbeets.com", Tier: 1, SearchURL: "https://myheartbeets.com/?s={q}", Trust: 0.9},
	{Name: "Indian Healthy Recipes", Hostname: "indianhealthyrecipes.com", Tier: 1, SearchURL: "https://www.indianhealthyrecipes.com/?s={q}", Trust: 0.9},
	{Name: "Sip and Feast", Hostname: "sipandfeast.com", Tier: 1, SearchURL: "https://www.sipandfeast.com/?s={q}", Trust: 0.9},

	// Tier 1 — re-added 2026-04-26 via Surf-Chrome impersonation. Plain Go
	// HTTP got 403/429 on these; Surf reaches them all with Recipe JSON-LD.
	// The Food52 search endpoint is /search?query={q} (their own SearchAction
	// target), not /recipes/search?q={q} which 404s. Note: Food52's *search*
	// page is JS-rendered (returns 0 permalinks), but `recipe get` works for
	// any known Food52 URL — kept here so the goat ranker has the option.
	//
	// Trust differentiation (2026-04-26): the four Dotdash-Meredith / mass-
	// market crowdsourced aggregators (AllRecipes, Food Network, Simply
	// Recipes, EatingWell) are weighted *below* the editorially-curated
	// sites. Same parent company owns AllRecipes, Simply Recipes, and
	// EatingWell, with no cross-site editorial curation. Recipes there are
	// user-submitted with light moderation. They serve as a fallback /
	// breadth signal — when an editorial site has the dish at a similar
	// rating, the editorial site wins on tie-break via the 0.15 site_trust
	// term in the goat ranker. Food Network is mid-tier (TV-chef brand
	// editorial, but recipe quality is mixed).
	{Name: "Food52", Hostname: "food52.com", Tier: 1, SearchURL: "https://food52.com/search?query={q}", Trust: 0.9},
	{Name: "AllRecipes", Hostname: "allrecipes.com", Tier: 1, SearchURL: "https://www.allrecipes.com/search?q={q}", Trust: 0.7},
	{Name: "Food Network", Hostname: "foodnetwork.com", Tier: 1, SearchURL: "https://www.foodnetwork.com/search/{q}-", Trust: 0.75},
	{Name: "Simply Recipes", Hostname: "simplyrecipes.com", Tier: 1, SearchURL: "https://www.simplyrecipes.com/search?q={q}", Trust: 0.7},
	{Name: "EatingWell", Hostname: "eatingwell.com", Tier: 1, SearchURL: "https://www.eatingwell.com/search?q={q}", Trust: 0.7},
	{Name: "Serious Eats", Hostname: "seriouseats.com", Tier: 1, SearchURL: "https://www.seriouseats.com/search?q={q}", Trust: 0.95},

	// Tier 2 — brand/editorial authority. Surf reaches all three; tier label
	// is a content-trust signal, not a reachability signal anymore.
	// Epicurious search URL fixed 2026-04-26: was /search/{q} (404); their
	// real search endpoint is /search?q={q}.
	{Name: "Bon Appétit", Hostname: "bonappetit.com", Tier: 2, SearchURL: "https://www.bonappetit.com/search?q={q}", Trust: 0.8},
	{Name: "Epicurious", Hostname: "epicurious.com", Tier: 2, SearchURL: "https://www.epicurious.com/search?q={q}", Trust: 0.8},
	{Name: "Gaz Oakley", Hostname: "gazoakleychef.com", Tier: 2, SearchURL: "https://www.gazoakleychef.com/?s={q}", Trust: 0.8},
}

// recipeURLPatterns is a lookup of compiled per-host regexes for candidate
// recipe-permalink paths. The Site.RecipeURLPattern field is populated from
// this map at init() time. Keys are the Site.Hostname (www-stripped).
// Patterns retained for removed sites (allrecipes, simplyrecipes,
// eatingwell, foodnetwork, food52) so that users who pass those URLs to
// `recipe get` still get correct site-level metadata (trust, tier).
// They're just absent from Sites so fan-out skips them.
var recipeURLPatterns = map[string]string{
	"kingarthurbaking.com":      `^/recipes/[a-z0-9-]+-recipe$`,
	"budgetbytes.com":           `^/[a-z0-9-]{6,}/?$`,
	"smittenkitchen.com":        `^/\d{4}/\d{2}/[a-z0-9-]+/?$`,
	"bbcgoodfood.com":           `^/recipes/[a-z0-9-]+$`,
	"bbc.co.uk":                 `^/food/recipes/[a-z0-9_]+_\d+$`,
	"minimalistbaker.com":       `^/[a-z0-9-]{6,}/?$`,
	"skinnytaste.com":           `^/[a-z0-9-]{6,}/?$`,
	"thekitchn.com":             `^/(?:.*-recipe-\d+|recipe-[a-z0-9-]+)$`,
	"recipetineats.com":         `^/[a-z0-9-]{6,}/?$`,
	"thewoksoflife.com":         `^/[a-z0-9-]{6,}/?$`,
	"justonecookbook.com":       `^/[a-z0-9-]{6,}/?$`,
	"thecozycook.com":           `^/[a-z0-9-]{6,}/?$`,
	"themediterraneandish.com":  `^/[a-z0-9-]{6,}/?$`,
	"kitchensanctuary.com":      `^/[a-z0-9-]{6,}/?$`,
	"grandbaby-cakes.com":       `^/[a-z0-9-]{6,}/?$`,
	"mykoreankitchen.com":       `^/[a-z0-9-]{6,}/?$`,
	"oliviascuisine.com":        `^/[a-z0-9-]{6,}/?$`,
	"feedthepudge.com":          `^/[a-z0-9-]{6,}/?$`,
	"preppykitchen.com":         `^/[a-z0-9-]{6,}/?$`,
	"sallysbakingaddiction.com": `^/[a-z0-9-]{6,}/?$`,
	"bromabakery.com":           `^/[a-z0-9-]{6,}/?$`,
	"biggerbolderbaking.com":    `^/[a-z0-9-]{6,}/?$`,
	"thecafesucrefarine.com":    `^/[a-z0-9-]{6,}/?$`,
	"chinasichuanfood.com":      `^/[a-z0-9-]{6,}/?$`,
	"redhousespice.com":         `^/[a-z0-9-]{6,}/?$`,
	"myheartbeets.com":          `^/[a-z0-9-]{6,}/?$`,
	"indianhealthyrecipes.com":  `^/[a-z0-9-]{6,}/?$`,
	"sipandfeast.com":           `^/[a-z0-9-]{6,}/?$`,
	"gazoakleychef.com":         `^/recipes/[a-z0-9-]+/?$`,
	"bonappetit.com":            `^/recipe/[a-z0-9-]+$`,
	"epicurious.com":            `^/recipes/food/views/[a-z0-9-]+$`,
	"seriouseats.com":           `^/[a-z0-9-]+-recipe(-\d+)?$`,

	// Removed from fan-out but URLs still recognizable for `recipe get`.
	// These sites either hard-block the search endpoint (Cloudflare on
	// omnivorescookbook), hard-block live reads (dotdash properties), or
	// ship client-rendered recipe pages (madewithlau, archanaskitchen —
	// Next.js SPAs we can't parse from Go without a headless browser).
	// Keeping the URL patterns means users who paste one of these URLs
	// into `recipe get` still get correct site-level metadata.
	"omnivorescookbook.com": `^/[a-z0-9-]{6,}/?$`,
	// Food52 uses two permalink shapes today:
	//   /recipes/89601-cosmopolitan-from-scratch  (numeric-id prefix)
	//   /recipes/sarah-fennel-s-best-lunch-lady-brownie-recipe  (slug only, 4+ parts)
	// Category pages like /recipes/dessert, /recipes/chicken, /recipes/quick-and-easy
	// also live under /recipes/ — the goat ranker filters those out via Recipe
	// JSON-LD validation, so the URL pattern just needs to admit candidates.
	"food52.com":        `^/recipes/(?:\d+-[a-z0-9-]+|[a-z0-9]+(?:-[a-z0-9]+){2,})$`,
	"foodnetwork.com":   `^/recipes/[a-z0-9-/]+-recipe-\d+$`,
	"allrecipes.com":    `^/recipe/\d+/[a-z0-9-]+/?$`,
	"simplyrecipes.com": `^/(?:recipes/[a-z0-9_-]+/?|[a-z0-9-]+-recipe-\d+)$`,
	"eatingwell.com":    `^/recipe/\d+/[a-z0-9-]+/?$`,
}

func init() {
	// Compile patterns once and back-fill onto the Sites entries.
	compiled := make(map[string]*regexp.Regexp, len(recipeURLPatterns))
	for host, pat := range recipeURLPatterns {
		compiled[host] = regexp.MustCompile(pat)
	}
	for i := range Sites {
		if re, ok := compiled[Sites[i].Hostname]; ok {
			Sites[i].RecipeURLPattern = re
		}
	}
}

// FindSite returns the Site matching the given hostname (www.-stripped), or
// an empty Site with Trust=0.5 when we don't recognize the host.
func FindSite(host string) Site {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	for _, s := range Sites {
		if s.Hostname == host {
			return s
		}
	}
	return Site{Hostname: host, Tier: 3, Trust: 0.5}
}

// siteByHost returns a pointer to the registered Site for the given host, or
// nil if the host is unknown. Kept separate from FindSite because callers that
// need the compiled regex benefit from sharing the struct verbatim.
func siteByHost(host string) *Site {
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	for i := range Sites {
		if Sites[i].Hostname == host {
			return &Sites[i]
		}
	}
	return nil
}

// BuildSearchURL applies url.QueryEscape to the query and substitutes into
// the site's SearchURL template. {q} is the only recognized placeholder.
func BuildSearchURL(site Site, query string) string {
	r := strings.NewReplacer("{q}", url.QueryEscape(query))
	return r.Replace(site.SearchURL)
}

// Author-trust scoring was removed in April 2026. The signal was largely
// redundant with site curation: every Tier 1 site is an independent blog
// where the author *is* the brand, and giving a bonus to named chefs on
// top of site curation double-counted the signal. It also created
// systematic noise — authors on curated sites whose byline format didn't
// match the curated entry (e.g. "Sally" vs "Sally McKenney" in JSON-LD)
// were silently penalized on their own blogs.
//
// If a cross-site author signal is reintroduced, prefer a boost tied to
// authors who appear on 3+ distinct sites rather than a static list.
