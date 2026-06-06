# Recipe GOAT CLI — Brief

## Pivot note

Initial target was `allrecipes-pp-cli`. Browser sniff confirmed the authenticated MyRecipes surface is gated by Dotdash Meredith TLS-fingerprint bot detection — not reachable from a plain Go HTTP client. The user pivoted: instead of fighting Dotdash, build a **cross-site recipe aggregator** that combines AllRecipes (anonymous tier-3 best-effort) with the rest of the quality recipe web. Product slug: **`recipe-goat`** (binary `recipe-goat-pp-cli`).

## Product Identity
- **Domain:** cross-site recipe aggregation, ranking, and local kitchen management.
- **Users:** home cooks and CLI-fluent power users who want "the best version of a recipe" without site-hopping, plus offline cookbook/meal-plan/shopping-list machinery.
- **Data profile:** Schema.org `Recipe` JSON-LD pulled from 15+ curated sites, merged into a local SQLite store with trust scoring, nutrition backfill via USDA FoodData Central, and a cook log only the user owns.

## The key insight
Every major recipe site publishes Schema.org JSON-LD for SEO. Every existing tool (recipe-scrapers, Paprika, Mealie, Tandoor, KitchenOwl) extracts it **per-site, in isolation**. None compare across sites. None weight by author trust. None merge reviews/nutrition/pairings. None combine free data-source APIs (USDA) with scraped recipes. The "GOAT" layer is the cross-site merge + trust model + local-store transcendence.

## Reachability tiers
| Tier | Sites | Behavior |
|------|-------|----------|
| 1 — curl-friendly | King Arthur, Budget Bytes, Smitten Kitchen, Food52, BBC Good Food, Minimalist Baker, SkinnyTaste, The Kitchn, Food Network | Plain GET + UA header; 99% success |
| 2 — Condé Nast | Bon Appétit, Epicurious | Moderate; occasional CAPTCHA on high volume |
| 3 — Dotdash hostile | AllRecipes, Simply Recipes, EatingWell, Serious Eats | 402/403 on automated requests; best-effort only, cache hits, document as optional |
| API — free auth | USDA FoodData Central (key needed, free, 3,500 req/hr), TheMealDB (no key) | Essential for nutrition backfill; use as first-class data sources |

## 2026-04-26 update — Surf-equipped HTTP unblocks Tier 3

Since the original 2026-04-13 brief, the printing-press generator (now v2.3.6) wires `github.com/enetx/surf` with Chrome impersonation into every emitted client (`Impersonate().Chrome()`). The April CLI predated this and used `net/http` directly, which is why Dotdash properties (TLS-fingerprint bot detection) returned 403/429.

**Re-probed 2026-04-26 against live sites** (raw output: `/tmp/printing-press/recipe-probe/results*.txt`):

| Site | Plain Go client | Surf-Chrome client | Decision |
|---|---|---|---|
| AllRecipes | 403 on `/`; 404 on recipes | 200 + Recipe JSON-LD on permalinks; search returns real `/recipe/<id>/<slug>/` permalinks | ✅ Re-added to Sites |
| Food52 | 429 on every URL | 200 + Recipe JSON-LD on real `/recipes/<id>-<slug>` permalinks; search HTML lacks recipe permalinks (JS-rendered) | ✅ Re-added; `recipe get`/`save` work; search returns 0 |
| Food Network | (was excluded) | 200 on homepage, search returns 18 permalinks under `/recipes/<slug>-recipe-<id>` | ✅ Re-added |
| Simply Recipes | (was excluded) | 200 on homepage, search returns recipe permalinks ending `-recipe-<numeric>`; recipe pages parse | ✅ Re-added |
| EatingWell | (was excluded) | 200 on homepage, search returns 14 permalinks under `/recipe/<id>/<slug>/`; recipe pages parse | ✅ Re-added |
| Serious Eats | (Tier 3 in registry) | 200 on homepage, search returns 16 real recipe permalinks | ✅ Promoted Tier 3 → Tier 1 |
| Epicurious | already in Tier 2 | search URL was wrong (`/search/{q}` 404s); fixed to `/search?q={q}` returns 12 permalinks | 🔧 SearchURL fixed |
| Bon Appétit, Gaz Oakley | already in Tier 2 | reachable | Unchanged |
| Smitten Kitchen | already 200 | already 200 | Unchanged (already in Tier 1) |

**Doctor's HEAD probe was lying.** Six sites (BBC Good Food, BBC Food, The Kitchn, RecipeTin Eats, AllRecipes, Serious Eats) reject `HEAD` requests with TLS shutdown / EOF, but serve `GET` 200 cleanly. Those sites were silently working in `goat`/`search` even though `doctor` reported them "unreachable". Doctor now uses `GET` with a `Range: bytes=0-1023` header.

**Implication for build priorities:** the same Surf client used by `internal/client/` for USDA must back the recipe site fetcher (`internal/recipes/fetch.go`). With Surf, every site previously labeled "Tier 3 hostile" is reachable; the Tier label is now a content-trust signal only, not a reachability signal. Site registry expanded from 28 to **37**.

The 2026-04-13 acceptance report's "Known limitation #1" (Dotdash 403/402) is closed. Live `goat "brownies"` returns 59 of 61 validated recipe candidates across 37 sites (was 51/52 across 28).

## Top Workflows
1. **"What's the best X recipe?"** — query across 15 sites, rank by trust × rating × review_count, return top 5 with provenance.
2. **"Get this recipe clean"** — given any URL, extract JSON-LD, strip ads/life-story, render as terminal card / markdown / print.
3. **"What can I cook tonight?"** — pantry-matched + time-bounded + skill-bounded search across the local cookbook.
4. **Weekly meal plan + shopping list** — plan per day, auto-aggregate shopping list with unit reconciliation.
5. **Cook log + learn my taste** — track what I actually cooked; re-rank future queries by my personal trust profile.

## Table Stakes
- Any-URL recipe extraction (match recipe-scrapers' 1,500+-site coverage via JSON-LD)
- Cross-site search with trust ranking
- Local cookbook store (SQLite) with FTS
- Nutrition + per-serving scaling
- Print / markdown / JSON / CSV output
- Doctor + API-key setup for USDA

## Data Layer
Primary entities: `sites`, `authors`, `recipes`, `recipe_clusters` (dedup groups), `ingredients` (parsed + USDA-matched), `instructions`, `reviews`, `nutrition_facts`, `substitutions`, `cook_log`, `meal_plans`, `shopping_list`, `costs`, `seasonal_ingredients`, `unit_equivalents`, `fetch_cache`.

Sync cursor: `fetched_at` per recipe; fetch_cache uses ETag/Last-Modified. FTS5 over recipe title + ingredient text + instructions + cook-log notes. Ingredient parser normalizes quantities + units + canonicalizes ingredient names against USDA FDC IDs.

## Auth model
- **No login required for core features.** Zero auth for Tier 1 / Tier 2 / Tier 3 site scraping.
- **Optional API key** for USDA FoodData Central (free; `doctor` prompts to set `USDA_FDC_API_KEY`). Enables nutrition backfill and ingredient-level macro precision.
- **No browser-mode / Chrome dependency.** Everything works over plain `http.Client`. Tier 3 sites may fail — the CLI reports this cleanly and falls back to cached data or other sources.

## Product Thesis
- **Name:** `recipe-goat`
- **Why it should exist:** No tool today ranks recipes across sites by merged trust + rating + review signal. No tool normalizes units across US / UK / metric sources. No tool backfills nutrition from USDA when a site omits it. No tool tracks which substitution suggestions come from which authoritative baker (King Arthur for buttermilk, Serious Eats for eggs, Budget Bytes for butter). The command-line audience is exactly the audience that wants machine-readable, aggregation-aware recipe tooling — not another web app. The local SQLite store unlocks compound queries (pantry match, stale-in-cookbook, cook-log regression) that no cloud app can offer without violating user privacy.

## Source Priority (multi-source aggregation, not ordinal)
This is a trust-weighted cross-site CLI, not a primary/secondary pipeline. `source-priority.json` captures the tiered reachability but every site is a peer in the aggregation layer. The README leads with the aggregation value prop, not a single site.

## Build Priorities
1. **Schema.org JSON-LD extractor** — generic, works for any site. Backbone of every other feature.
2. **Site adapters + search URL patterns** — 15 sites, each with a search URL template and JSON-LD quirks documented.
3. **Local cookbook store** — recipes, ingredients, nutrition, substitutions; FTS5 indexes.
4. **Cross-site search + ranking** — multi-site fan-out, dedup clusters, trust scoring.
5. **USDA nutrition backfill** — match parsed ingredients to FDC IDs, compute per-serving when missing.
6. **Cook log, meal plan, shopping list** — local-first, aggregation-aware.
7. **Trust model** — author/site weights; user-tunable; regression learning from cook log.

## Reachability Mitigations
- Global `--rate-limit` (default 1 req/sec per host, concurrency 4 across hosts).
- ETag/Last-Modified cache on every fetch; SQLite-backed.
- Tier 3 sites: single retry with backoff, then graceful fallback with clear messaging ("Tier 3 sites unavailable — your query ranked `X`, `Y`, `Z` from `A`, `B`, `C` sites").
- `doctor` reports per-site reachability and documents the known Dotdash limitation.
