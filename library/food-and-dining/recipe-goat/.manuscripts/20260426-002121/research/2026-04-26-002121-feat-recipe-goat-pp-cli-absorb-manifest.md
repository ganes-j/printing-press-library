# Recipe GOAT — Absorb Manifest (v2)

Slug: `recipe-goat` · Binary: `recipe-goat-pp-cli`

## User-first rewrite

v1 of this manifest had features that "sounded good" (technique consensus, recipe-drift detection, personal-taste regression). A home cook doesn't care about any of that. v2 starts from **four real personas**, maps their actual rituals and frustrations, then only keeps features that directly serve them.

### The four personas

| Persona | Who | Rituals | Top frustrations |
|---|---|---|---|
| **Beginner** | Cooking more; doesn't know subs; afraid to deviate | Googles "easy [thing]"; clicks first result; follows exactly | "What do I substitute for X?" · "Is this recipe any good?" · "How much salt is right?" |
| **Weeknight** | Intermediate; cooks 4-5×/wk for household | Monday-night planning; opens fridge; searches by time budget | "What can I make with this chicken?" · "Plan 5 dinners without my kids rebelling" · "Shopping list that aggregates quantities" |
| **Parent/budget** | Feeding 2+; time-poor; cost-aware | Batch cooking; freezer meals; leftover Tetris | "Kid-friendly filter that isn't garbage" · "What's cheap this week?" · "Don't make the same thing three weeks running" |
| **Enthusiast** | Cooks for joy; keeps notes; has opinions | Follows specific chefs; collects techniques; weekend projects | "Is this the best X online?" · "What did reviewers actually change?" · "Trust Kenji over some AR user named BakerMom47" |

Every transcendence feature below is tagged with the personas it serves. If a feature serves zero personas, it's out.

---

## Tools catalogued

Active: **hhursev/recipe-scrapers** (Python, 400+ sites). Storage apps: **Paprika**, **Mealie**, **Tandoor**, **KitchenOwl**. Markup: **Cooklang**. Stale AR-only CLIs: **marcon29/CLI-dinner-finder**, **python-allrecipes**, **mdimec4/allrecipes**. Commercial: **Apify**. Paid APIs: **Edamam**, **Spoonacular** (skipped). Free APIs: **USDA FoodData Central**, **TheMealDB**, **OpenFoodFacts**.

Every existing tool is either a per-site extractor or a single-URL storage app. None aggregate, dedup, or trust-score across sites. None integrate USDA for nutrition backfill.

---

## Absorbed (match or beat everything that exists) — 22 features

| # | Feature | Best Source | Our Implementation | Personas |
|---|---|---|---|---|
| 1 | Extract any recipe URL | recipe-scrapers | `recipe get <url>` — JSON-LD parser | all |
| 2 | Multi-site support | recipe-scrapers | Same; plus per-site search adapters for 15 curated sites | all |
| 3 | Search a single site | site-specific tools | `search <q> --site food52` | weeknight, enthusiast |
| 4 | Scale servings | Paprika, AR | `recipe get <url> --servings 6` (rational fractions, unit-aware) | all |
| 5 | Unit convert | Paprika | `recipe get <url> --units metric\|us` | weeknight, enthusiast |
| 6 | Print-friendly | RecipeStripper | `recipe get <url> --print` (no ads, no life-story) | all |
| 7 | Markdown export | Mealie | `recipe get <url> --md` | enthusiast |
| 8 | Save to cookbook | Paprika, Mealie | `save <url>` → SQLite; bulk via stdin | all |
| 9 | List saved | all managers | `cookbook list` with `--json\|--tag\|--site\|--author` | all |
| 10 | Remove saved | all managers | `cookbook remove <id> --dry-run` | all |
| 11 | Tag saved | Paprika | `cookbook tag <id> weeknight,comfort` | weeknight, enthusiast |
| 12 | Ingredient filter | Paprika | `cookbook search --with "chicken,rice" --without shellfish` | all |
| 13 | Cook log | Paprika (manual) | `cook log <id> --rating 4 --notes "too salty"` | enthusiast, weeknight |
| 14 | Meal plan | Mealie | `meal-plan set 2026-04-15 dinner <id>` + `meal-plan show --week` | weeknight, parent |
| 15 | Dietary filter | Mealie | `search <q> --vegan --gluten-free` (verified vs allergen dict) | all |
| 16 | Import from other tool | Mealie, Paprika | `cookbook import --format paprika\|mealie\|cooklang` | enthusiast |
| 17 | Export to other tool | — | `cookbook export --format ...` (escape the walled garden) | all |
| 18 | Open in browser | Paprika | `recipe open <id>` | all |
| 19 | Bulk scrape | recipe-scrapers CLI | `save --stdin < urls.txt` with rate limit + concurrency | enthusiast |
| 20 | FTS local search | Mealie | `cookbook search <q>` (FTS5 on title + ingredients + instructions + notes) | all |
| 21 | Trending / today | per-site homepages | `trending [--site ...]` snapshot | weeknight, enthusiast |
| 22 | Doctor | standard | `doctor` — checks USDA key, per-site reachability, SQLite | all |

## Transcendence — 10 features, each tied to a persona job

Before you read these: v1 had 15. I cut 5 because they failed the "actually useful to home cooks" test. What's dropped and why is at the bottom.

| # | Feature | Command | Persona job it does | Evidence | Score |
|---|---|---|---|---|---|
| T1 | **Best-version ranker** | `goat "chicken tikka masala"` | Beginner: "is this recipe good?" Enthusiast: "is this *the* best?" Returns top 5 across 15 sites, ranked by `normalized_rating × log(review_count) × author_trust × site_trust × recency`. Shows source + rating + review count + total time per result. | #1 recipe-site complaint across Reddit/HN: "I googled 'chicken tikka masala' and got 10 blogs with 2,000-word essays." | 10/10 |
| T2 | **Substitution lookup** | `sub buttermilk` | Beginner hits this weekly: "I'm out of X, what works?" Aggregates subs from King Arthur, Serious Eats, AR reviews, Budget Bytes; ranks by source trust; shows ratio + context ("in baking: milk + lemon juice; in marinades: yogurt"). | Subs are scattered tribal knowledge; no unified tool exists. Beginners fear deviating without guidance. | 10/10 |
| T3 | **Pantry match** | `cookbook match --have "chicken,rice,broccoli" --missing-max 2` | Weeknight + parent: "what can I make with what's in my fridge?" Local join against cookbook with ingredient canonicalization; shows recipes you could make now, or with ≤2 missing ingredients (listed). | #1 request on r/Cooking "what can I make with..." threads. No web tool solves it because it needs your pantry data locally. | 10/10 |
| T4 | **Tonight picker** | `tonight --max-time 30m --no-repeat-within 7d [--kid-friendly]` | Weeknight: "it's 5 pm, what am I making?" Pulls from cookbook; filters by time, recency-from-cook-log, dietary; returns 3 candidates in 2 seconds. | Decision fatigue is the #1 weeknight-cook complaint. Kills the 20-minute "what are we having?" debate. | 9/10 |
| T5 | **Review-modification digest** | `recipe reviews <id>` (or inline with `recipe get --reviews`) | All personas, especially beginner: "what did other cooks actually change?" Heuristic keyword extraction over review bodies surfaces "added an egg: 22 cooks; baked 5 min less: 17; used honey instead of sugar: 14." | Top-level tribal knowledge hidden in 500-review threads. The hack that turns a 4-star recipe into a 5-star one. | 9/10 |
| T6 | **USDA nutrition backfill** | implicit in `recipe get`, `recipe get --nutrition` | Health-conscious cooks want accurate macros. When JSON-LD lacks `nutrition`, parse ingredients, match USDA FDC IDs, compute per-serving. Marks nutrition as `[source: site]` or `[source: USDA-computed]` so the user knows the provenance. | ~30% of recipes across our 15 sites omit nutrition; SkinnyTaste / WW cooks live by macros. Free, authoritative fallback. | 8/10 |
| T7 | **Kid-friendly filter** | `search <q> --kid-friendly` + `tonight --kid-friendly` | Parent: "my 6-year-old won't eat capers." Ingredient-based filter keyed off an editable exclusion list (anchovies, capers, raw onions, excess heat, unusual cuts). User can `kid-list add <ingredient>` to personalize. | No recipe site has a useful kid-friendly filter — existing ones show white-bread pasta dishes only. Ingredient exclusion is the honest way. | 8/10 |
| T8 | **Unit-reconciling shopping list** | `meal-plan shopping-list --week` | Weeknight + parent: "what do I buy?" Aggregates across planned meals; reconciles "2c + 1c milk → 3c"; groups by grocery aisle (produce / meat / pantry / dairy / frozen). `--export md\|txt\|csv`. | Everyone makes a shopping list; everyone fails at unit math. No CLI does this right; Mealie/Tandoor do it but they're web apps. | 8/10 |
| T9 | **Seasonal flag (inline)** | auto in `recipe get` output | All personas: quality + cost both improve in-season. "⚠ asparagus is typically out of season in November (peak: April–June in US)." Not preachy; one line; `--no-seasonal` to suppress. Can also `search <q> --in-season`. | Strawberries-in-January is universally "meh." Useful signal, low-complexity local table (NOAA + USDA data, region × month). | 7/10 |
| T10 | **Cost-aware display** | `recipe get <url> --cost` + `meal-plan cost --week` | Budget/parent: "rough cost per serving?" Budget Bytes has line-item cost for its own recipes; for others, ingredient → USDA retail average (free public dataset) → rough estimate with honesty band ("$6–$9 for 4 servings, ±30%"). | Budget is real; precision is impossible. Better than silence. Simple heuristic beats no heuristic. | 7/10 |

**All 10 features have a concrete persona, a concrete job, and a concrete evidence pointer.** Each answers a question the user actually has.

### Dropped from v1 and why

- **Technique consensus** — "7/10 top results whisk sugar into hot butter." Sounded elegant. A beginner clicking a top-ranked recipe already gets the technique; they don't need meta-analysis. **Cut.**
- **Recipe drift detector** — "The recipe was silently edited on AR." Paranoid. Home cooks don't care. **Cut.**
- **Personal trust regression** — "Learn per-user author weights from cook log." Requires dozens of ratings to be useful; most cooks don't log religiously. Overengineered. **Cut.**
- **Stale-in-cookbook** — "What have I saved but not cooked in 60 days?" Niche to power users with 300-recipe cookbooks. Most cooks save 20–40 recipes they cook regularly. **Cut.**
- **Pairing composer** — "Entrée + wine + side + dessert from four sites." Delightful but low-frequency; solves a dinner-party problem not a Tuesday problem. **Cut.**
- **Cost-optimized meal plan solver** — "7 dinners under $80." The constraint-solver complexity is huge; the cost data is noisy; the output would feel fragile. Replaced with T10 (honest cost estimate that doesn't over-promise).

### Plumbing (exists but isn't user-facing)

- Cross-site dedup (enables T1)
- Author trust graph (enables T1; `trust list`, `trust set kenji +2` for enthusiasts who want control)
- Site trust tiers (Tier 1/2/3 reachability from source-priority.json)
- Fetch cache with ETag/Last-Modified
- Ingredient parser + USDA FDC matcher
- Unit equivalents table (ingredient-aware: 1 cup flour ≠ 1 cup sugar in grams)

These are mentioned so the engineering scope is honest; they're not features in the user-facing CLI surface.

---

## Totals

- **22 absorbed features** — match or beat every existing recipe tool
- **10 transcendence features** — each tied to a real home-cook job, each with concrete evidence
- **32 user-facing commands** — down from 40 in v1 but substantially more *useful*
- **Sources:** 15 recipe sites (tier-1 curl-friendly, tier-2 Condé Nast, tier-3 Dotdash best-effort) + USDA FoodData Central + TheMealDB
- **No paid APIs. No headless Chrome. No login required for core features.**

---

## 2026-04-26 update — site list expansion (final)

Since the original 2026-04-13 manifest, the printing-press generator now embeds `github.com/enetx/surf` with Chrome impersonation in every emitted HTTP client. Surf bypasses TLS-fingerprint bot detection that previously blocked Dotdash properties.

**Initially planned**: re-add the user's three target sites — AllRecipes, Food52, Smitten Kitchen.

**Actually delivered after re-probe**: every Tier 3 / "removed" site Surf could reach. The user's question "do tier 2/3 truly not work even with surf" surfaced systemic outdated information: Surf reaches **all 37 sites** in the registry today.

**Sites added or promoted (tested live 2026-04-26 with Surf-Chrome):**

| Site | Status before | Status now | Search results (brownies) |
|---|---|---|---|
| AllRecipes | excluded (Dotdash 403) | Tier 1 | 9+ real permalinks |
| Food52 | excluded (CDN 429) | Tier 1 | 0 (search is JS-rendered; `recipe get`/`save` work fine) |
| Food Network | excluded (429) | Tier 1 | 18 permalinks |
| Simply Recipes | excluded (Dotdash) | Tier 1 | 3+ permalinks, recipe pages parse |
| EatingWell | excluded (Dotdash) | Tier 1 | 14 permalinks, recipe pages parse |
| Serious Eats | Tier 3 (best-effort) | Tier 1 | 16 permalinks |
| Epicurious | Tier 2 with broken search URL | Tier 2 with `/search?q={q}` | 12 permalinks |
| Smitten Kitchen | Tier 1 | Tier 1 (unchanged) | works as before |

**Site count: 28 → 37.**

**Doctor probe bug fixed**: doctor was using `HEAD` for site reachability but six sites (BBC Good Food, BBC Food, The Kitchn, RecipeTin Eats, AllRecipes, Serious Eats) reject HEAD with TLS shutdown / EOF while serving GET 200 cleanly. Doctor now uses `GET` with `Range: bytes=0-1023` so headers come back without pulling the whole page.

**Tier label semantics changed**: the Tier 1/2/3 split was a reachability hierarchy in the pre-Surf world. With Surf in the transport, every site is reachable; tier is now a content-trust signal only.

**Live verification**: `goat "brownies"` returns 59 of 61 validated recipe candidates across the expanded registry (was 51/52 across the 28-site baseline).

No new transcendence features. No new commands. The 6 added/promoted sites flow through the existing fan-out (`goat`, `search --site <host>`, `recipe get <url>`, `save <url>`) and the same dedup/trust pipeline.
