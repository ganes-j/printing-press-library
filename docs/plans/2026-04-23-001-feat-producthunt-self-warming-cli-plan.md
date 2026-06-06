---
title: "feat(producthunt): self-warming CLI with Atom auto-sync and GraphQL enrichment"
type: feat
status: active
date: 2026-04-23
---

# feat(producthunt): self-warming CLI with Atom auto-sync and GraphQL enrichment

**Target repo:** `printing-press-library` (all paths below are relative to the repo root).

## Overview

Make `producthunt-pp-cli` keep itself warm so integrators never have to think about cold stores. Three layers, smallest first:

1. **Auto-sync on stale reads (Tier 1, always-on, no auth).** Any read command (`search`, `list`, `recent`, etc.) that finds the local store >24h stale runs `sync` internally before serving the query. Token-free. Free.
2. **Topic-aware GraphQL enrichment (Tier 2, opt-in with `--enrich`).** When a read returns thin results and the user has OAuth credentials configured, the CLI fires a narrow GraphQL query for that specific topic over the last 30 days and upserts before re-querying the store. Bounded, budget-aware.
3. **Explicit backfill (Tier 3, `backfill` subcommand).** One-shot 30-day historical seed for cold-start or gap recovery, plus `backfill resume` for interrupted runs.

Design principle: **the CLI owns its freshness.** Integrators (last30days and future callers) should get good results from `producthunt-pp-cli search "PostHog"` without having to know about sync cadence or store warmth. The CLI decides when to talk to the network.

This plan keeps the Atom-first design of PR #112 for the default read path — Tier 1 is strictly additive and needs no auth. Tiers 2 and 3 are opt-in and only activate when the user has registered an OAuth app.

---

## Problem Frame

Today's `producthunt-pp-cli` is Atom-feed-only and token-free. The Atom feed exposes the 50 most-recently-featured launches, which amounts to a rolling 1-3 day window. This is fine for steady-state sync but makes the CLI nearly useless the first time it's used for any query older than a couple of days. Confirmed in the 2026-04-23 last30days feasibility study: queries for "PostHog", "June Oven", and most of Matt's recent real queries returned empty because the matching PH launches are older than today's 50-post window.

**The CLI should own its own freshness.** Putting that logic in every integrator (last30days today, future callers tomorrow) duplicates the same staleness check across skills that shouldn't have to know about it. The cleaner split: callers ask `producthunt-pp-cli` for data on a topic; the CLI decides whether it needs to re-sync or fetch new data before responding. Integrators stay dumb and the CLI stays smart.

A CLI that knows "I was called and my store is stale" can:

1. **Auto-sync on stale reads.** Any `search`/`list`/`recent` run with `ph_meta.last_sync_at` older than 24h silently calls `sync` first. This is free (Atom, no auth) and covers the normal case — users who run last30days daily keep their store warm as a side effect.
2. **Enrich thin topic results via GraphQL.** When a user runs `search "PostHog" --enrich` and the local store has fewer than N results, the CLI fires a narrow GraphQL query (`posts` where tagline/name/slug contains "PostHog", last 30 days) and upserts before re-querying. Opt-in per call or via `config.toml`.
3. **Explicit backfill for bulk cold-start.** First-time installs want 30 days of coverage immediately, not 7 days from now after sync-on-read warms the store. Same for gap recovery after a week offline. Same for "give me everything around event X." These cases need the `backfill` subcommand.

PH's GraphQL API has full historical data but was intentionally skipped in PR #112 because continuous streaming blows through the 6,250-complexity-points-per-15-minute budget. However, a one-shot 30-day backfill is ~300-400 complexity points — roughly 7% of a single 15-minute window. The budget is only "impractical" for streaming; a time-bounded backfill fits comfortably.

The win: after one ~90-second backfill, every store-backed command has 30 days of data to work with immediately. Sync-on-invoke (in the integrator) keeps it warm from there.

---

## Requirements Trace

### Tier 1: auto-sync-on-read (always on, no auth)

- R1. Any read command (`search`, `list`, `recent`, `today`, `trend`, `calendar`, `makers`, `tagline-grep`, `outbound-diff`) checks `ph_meta.last_sync_at` before serving the query. If >24h stale (or no sync has ever run), call `sync` internally, then serve.
- R2. Auto-sync is observable: read-command output in `--json` mode includes `_meta.auto_synced: {ran: true|false, posts_upserted: N, elapsed_ms: M}` so integrators can see what happened.
- R3. Auto-sync is disable-able via `--no-auto-sync` flag (per-call) and `auto_sync = false` in `config.toml` (global).
- R4. A new optional `--caller <identifier>` flag lets integrators identify themselves (e.g. `--caller last30days/3.0.1`) for logging and debugging.

### Tier 2: topic-aware GraphQL enrichment (opt-in)

- R5. A new `--enrich` flag on `search` triggers a narrow GraphQL query when the local store has fewer than N results (default N=3) for the topic. Requires `auth.type == "oauth"`. Silently skipped if no OAuth configured.
- R6. Enrichment query covers the last 30 days by default, bounded to one paginated request (≤20 posts) per invocation. Budget impact capped at ~5 complexity points per enriched call.
- R7. Enrichment is opportunistic: failure to enrich (rate limit, network, auth) does not fail the underlying read. The user always gets whatever the store has, with a `_meta.enrich_attempted` note in JSON output.

### Tier 3: explicit backfill (user-initiated bulk seed)

- R8. A new `producthunt-pp-cli auth register` flow that walks the user through registering an OAuth app on Product Hunt, performs the `client_credentials` exchange, and persists the resulting token.
- R9. A new `producthunt-pp-cli backfill [--days N]` subcommand that paginates the PH GraphQL `posts` query over the requested window and upserts results into the existing SQLite store.
- R10. A `producthunt-pp-cli backfill resume` subcommand that continues an interrupted backfill from the last saved cursor without duplicating work or burning budget on already-fetched pages.
- R11. Backfill must stay under 50% of the 6,250-complexity-point-per-15-minute budget per run; respect `X-Rate-Limit-Remaining` on every response; soft-brake at 25% remaining; hard-stop at 10% remaining and save the cursor.
- R12. Backfill must emit a `--dry-run` mode that estimates complexity cost and prints the planned request pattern without making any GraphQL calls.

### Cross-tier

- R13. OAuth credentials must live in the existing `config.toml` path (0600 perms), not the SQLite store, and must be independently revocable via `auth logout`.
- R14. The existing Atom runtime (`sync`, `recent`, `today`, `feed`, `list`, `search`) stays semantically unchanged — same inputs, same outputs. Auto-sync adds a pre-step but does not alter results.
- R15. Documentation: `SKILL.md`, `README.md` updated so integrators understand "just call search, the CLI handles the rest" is the intended posture.

---

## Scope Boundaries

- Not replacing the Atom-first runtime. Atom stays primary; GraphQL is strictly for the one-shot backfill.
- Not implementing continuous GraphQL polling. Budget math does not close for steady-state streaming; attempting it would eventually trip rate limits.
- Not implementing sync-on-invoke — that belongs in the integrator (e.g. `last30days-skill`'s PH source adapter), not in this CLI. Integrators check if the store is stale (>24h since last `sync`) at the top of their run and call `producthunt-pp-cli sync` if so. That's the "cron" story for steady-state warmth. This plan solves the complementary problems (cold-start, gap-recovery, narrow-window backfill) that sync-on-invoke cannot reach.
- Not implementing OAuth `authorization_code` flow. `client_credentials` is sufficient for app-level reads of public data and avoids the browser-redirect complexity.
- Not building views on top of backfilled data beyond what the existing store-backed commands already deliver. If new transcendence views shake out as useful post-backfill, they are a follow-up plan.
- Not modifying `spec.yaml`'s `kind: synthetic` declaration. Backfill lives entirely in hand-written `ph_*.go` files, same as the existing Atom extensions.
- Not bypassing Cloudflare-gated routes (`post`, `comments`, `leaderboard`, etc.). Those stay stubs. Backfill uses only the official GraphQL surface.

### Deferred to Follow-Up Work

- Scheduled daily sync (cron/launchd) integration: the steady-state warmup story, documented separately.
- Backfill-sourced engagement signal boost in last30days adapter: separate PR in last30days-skill once backfill is merged and producing real data.
- PH API v3 migration: if PH ships a v3 endpoint later, re-evaluate the backfill path. The current plan targets v2.

---

## Context & Research

### Relevant Code and Patterns

- `library/marketing/producthunt/internal/cli/auth.go` — existing cobra-based auth subcommand tree (`status`, `set-token`, `logout`). Pattern for extending with `register` subcommand.
- `library/marketing/producthunt/internal/config/config.go` — `Load()`, `AuthHeader()`, `AuthSource` already handle TOML-based token persistence. Extend for OAuth client creds + app token.
- `library/marketing/producthunt/internal/store/ph_ext.go` — Hand-written PH-specific schema extensions. `PHTablesSchemaVersion` constant is the migration epoch. Adding a backfill state table means bumping to version 2 and writing the migration.
- `library/marketing/producthunt/internal/cli/sync.go` — Existing paginated-upsert pattern for Atom snapshots. Backfill's upsert loop should mirror this structure (read source, upsert into `posts` + `snapshot_entries`, update `ph_meta`).
- `library/marketing/producthunt/internal/client/client.go` — Existing HTTP client with the `enetx/surf` stack. GraphQL client layers on top of the same transport.
- `library/marketing/producthunt/internal/cli/ph_feed.go`, `ph_types.go` — Naming convention for hand-written extensions that survive Printing Press regeneration. New GraphQL code must follow the `ph_*` prefix rule.
- `.manuscripts/` directory — Printing Press convention for discovery notes and proofs. The backfill work should generate a brief `.manuscripts/` record alongside the PR.

### Institutional Learnings

- `.claude/projects/-Users-mvanhorn/memory/feedback_pp_go_install_goprivate.md` — GOPRIVATE prefix required when installing PP CLIs.
- `.claude/projects/-Users-mvanhorn/memory/feedback_pp_update_before_run.md` — Always `go install @latest` before running PP CLIs.
- `PR #112` retro (linked from PR body): generator boilerplate in `README.md` and `SKILL.md` has historically shipped with hallucinated sections. The docs update in U6 must be reviewed against this class of failure.

### External References

- Product Hunt API v2 docs: https://api.producthunt.com/v2/docs (GraphQL schema, auth flows)
- OAuth 2.0 `client_credentials` grant: https://www.rfc-editor.org/rfc/rfc6749#section-4.4
- PH complexity budget: 6,250 points per 15-minute rolling window, returned via `X-Rate-Limit-*` response headers

---

## Key Technical Decisions

- **Client_credentials grant over authorization_code.** App-level read of public data is sufficient for backfill; no user-specific scopes are needed. Avoids the browser-callback complexity that would otherwise require a local HTTP server during `auth register`.
- **One-shot design; no streaming.** Backfill is idempotent and resumable but not repeatable on a schedule. After the first success, the user never needs to touch GraphQL again unless they widen the window. This keeps budget math trivial.
- **Budget-aware pagination, not fixed-rate.** Read `X-Rate-Limit-Remaining` on every response; adjust inter-page sleep dynamically (100ms when >50% budget remains, 300ms at 25-50%, hard stop below 10%). This is both more polite and more efficient than a blind fixed sleep.
- **Cursor persistence after every page, not at end.** If a 500-page backfill crashes on page 497, the user should resume from 497, not from zero. State lives in a new `ph_backfill_state` table keyed by window (from/to dates).
- **Minimal GraphQL field set.** Query only `id, slug, name, tagline, createdAt, url, votesCount, user { name username }`. Each field adds complexity cost; dropping comments/topics/reviews cuts per-page cost roughly in half.
- **User-Agent explicitly identifies the client.** `producthunt-pp-cli/<version> (+github.com/mvanhorn/printing-press-library)`. Unidentified clients get more scrutiny from API teams; identified ones are easier to whitelist and debug.
- **OAuth creds in config.toml, not the SQLite db.** SQLite files get shared, backed up, or copied across machines more readily than the TOML config. Secrets belong in 0600-permission TOML.
- **`--dry-run` computes a concrete estimate, not a hand-wave.** The estimate reads the same complexity-cost formula the real query will use, so the number printed is the number the user will pay. No surprise bills.
- **Upsert semantics via `INSERT ... ON CONFLICT(post_id) DO UPDATE`.** Backfill and Atom sync both target the same `posts` table; overlap on recent posts is expected and desirable. Prefer the fresher record when `updated_at` differs.

---

## Open Questions

### Resolved During Planning

- **Which OAuth grant?** client_credentials — sufficient for public data, avoids browser redirect.
- **Where does the token live?** config.toml, extending the existing `auth.go` flow, not the SQLite store.
- **Which GraphQL query surface?** `posts` with `postedAfter`/`postedBefore`/cursor pagination, NEWEST order, minimal fields.
- **Does backfill need its own schema version bump?** Yes — `PHTablesSchemaVersion` goes from 1 to 2 to add `ph_backfill_state`. Schema migrate step runs in `EnsurePHTables()`.
- **How is overlap with Atom-synced posts handled?** Upsert, preferring fresher `updated_at`. No duplicate-detection logic needed.

### Deferred to Implementation

- Exact complexity cost per page — PH's published formula is approximate; the dry-run estimator should read the actual `X-Rate-Limit-*` response from a single canary query to calibrate. Done during U4.
- Whether `votesCount` at backfill time matches live values — PH may snapshot votes at query time or cache for minutes. Observe during U4 testing; if inconsistent, note in the docs but don't gate the ship on it.
- Whether `createdAt` in GraphQL matches the Atom `<published>` timestamp exactly — both are ISO8601 UTC per PH docs, but edge cases may exist. Normalize to UTC date-only (`YYYY-MM-DD`) during upsert to sidestep the issue.
- Resume cursor format: opaque string from PH's `pageInfo.endCursor` vs. a composite `{postedAfter, lastPostID}` tuple. PH's opaque cursor is simpler but couples resume to PH's pagination implementation; the composite is more resilient to PH-side cursor rotation. Pick during U5 based on what PH actually returns.

---

## High-Level Technical Design

> *This illustrates the intended approach and is directional guidance for review, not implementation specification. The implementing agent should treat it as context, not code to reproduce.*

```
auth register flow:
  user -> "auth register"
       -> CLI prints: "open https://www.producthunt.com/v2/oauth/applications
                      register an app, paste client_id and client_secret below"
       -> reads client_id, client_secret from stdin (no-echo for secret)
       -> POST /v2/oauth/token  {grant_type: client_credentials, ...}
       -> receives access_token
       -> writes config.toml: auth.type = "oauth", auth.token = <access_token>
       -> prints: "authenticated as app <client_id prefix>"

backfill flow:
  user -> "backfill --days 30 [--dry-run]"
       -> load config.toml, require auth.type == oauth
       -> compute window: [now - 30d, now]
       -> if --dry-run: print estimated pages, complexity, expected elapsed, exit 0
       -> open store, EnsurePHTables, check ph_backfill_state for existing window
       -> if resuming: load cursor, log "resuming at cursor <X>"
       -> loop:
            check budget: if X-Rate-Limit-Remaining < 625 (10%), save cursor, exit 3
            if 25% < remaining < 50%, sleep 300ms
            GraphQL posts query with cursor
            upsert results into posts + ph_backfill_state.cursor
            if no more pages, mark window complete, exit 0

resume flow:
  user -> "backfill resume"
       -> load config, load ph_backfill_state
       -> if no pending window: "nothing to resume", exit 0
       -> if schema version mismatch: "store migrated since backfill, aborting", exit 1
       -> else: re-enter backfill loop from saved cursor
```

---

## Implementation Units

- [ ] U1. **OAuth register command and config extension**

**Goal:** Let a user run `producthunt-pp-cli auth register`, paste their PH app's client_id and client_secret, get a working access_token stored in `config.toml`.

**Requirements:** R1, R6.

**Dependencies:** None.

**Files:**
- Create: `library/marketing/producthunt/internal/cli/ph_auth_register.go`
- Modify: `library/marketing/producthunt/internal/cli/auth.go` (wire new subcommand into tree)
- Modify: `library/marketing/producthunt/internal/config/config.go` (add `AuthType` / `OAuth` fields)
- Test: `library/marketing/producthunt/internal/cli/ph_auth_register_test.go`

**Approach:**
- New file `ph_auth_register.go` (ph_ prefix preserves it through PP regeneration).
- `auth register` walks the user to the PH OAuth apps page via printed URL, prompts for client_id (visible) and client_secret (no-echo via `golang.org/x/term`).
- Performs `client_credentials` token exchange against `https://api.producthunt.com/v2/oauth/token`.
- On success, writes token to config.toml with `auth.type = "oauth"` and 0600 perms.
- On failure, prints PH's error message verbatim and exits 1. Do not retry automatically.
- Extend `auth status` to show OAuth vs. none (backward compatible: existing installs keep working).

**Patterns to follow:** `auth.go:newAuthSetTokenCmd`, `config.go:AuthHeader`.

**Test scenarios:**
- Happy path: valid client_id/secret → token exchange succeeds → config.toml written with token and `auth.type = "oauth"`.
- Error path: invalid client_secret → PH returns 401 → CLI prints PH error message, exits 1, config.toml untouched.
- Error path: network timeout → CLI prints "network error, no token saved", exits 1.
- Edge case: existing config.toml with `auth.type = "none"` → register overwrites auth section only, preserves other config.
- Edge case: config.toml permissions — after write, file mode must be 0600.
- Integration: `auth status` after successful register reports `Source: oauth` instead of `Source: (none)`.

**Verification:** All six test scenarios pass. Manual run: `producthunt-pp-cli auth register` with a real PH app works end-to-end on a fresh install.

---

- [ ] U2. **GraphQL client with budget tracking**

**Goal:** A reusable Go package that issues PH GraphQL queries, reads rate-limit headers on every response, exposes remaining-budget as a public value, and handles 429s gracefully.

**Requirements:** R4.

**Dependencies:** U1 (needs a working OAuth token to test against).

**Files:**
- Create: `library/marketing/producthunt/internal/phgraphql/client.go`
- Create: `library/marketing/producthunt/internal/phgraphql/budget.go`
- Create: `library/marketing/producthunt/internal/phgraphql/queries.go` (raw GraphQL strings as Go consts)
- Test: `library/marketing/producthunt/internal/phgraphql/client_test.go`

**Approach:**
- New `phgraphql` package to cleanly isolate GraphQL concerns from the Atom runtime in `client.go`.
- Client wraps the existing `enetx/surf` HTTP stack for consistent TLS fingerprint.
- `User-Agent: producthunt-pp-cli/<version> (+github.com/mvanhorn/printing-press-library)` header on every request.
- Per-response: parse `X-Rate-Limit-Limit`, `X-Rate-Limit-Remaining`, `X-Rate-Limit-Reset` into a `Budget` struct. Expose `Budget()` getter on the client.
- On 429: parse `Reset` header, return a typed `RateLimitedError{ResetAt: time.Time}` so callers can decide retry vs. save-and-exit.
- On any non-2xx: return typed error with status code and response body snippet.
- Single `Execute(ctx, query, variables)` method; caller owns pagination.

**Execution note:** Implement test-first. The budget math and 429 handling are the parts most likely to ship broken. Mock HTTP responses with realistic PH rate-limit headers; assert budget field updates correctly and that 429s return the right error type.

**Patterns to follow:** `internal/client/client.go` for HTTP transport setup.

**Test scenarios:**
- Happy path: successful query → response parsed → `Budget.Remaining` reflects header value.
- Happy path: first query has no prior budget state → `Budget()` returns zero values until first response processed.
- Edge case: response missing `X-Rate-Limit-*` headers → `Budget` remains at last known values, warning logged.
- Edge case: `X-Rate-Limit-Remaining` is malformed → treat as unknown, fall back to conservative defaults.
- Error path: 429 response → returns `RateLimitedError` with `ResetAt` populated from header.
- Error path: 401 response → returns typed `AuthError`; caller knows to instruct user to re-register.
- Error path: network timeout → returns context-deadline error; caller handles retry policy.
- Integration: User-Agent header present on every outbound request (capture with test server).

**Verification:** All eight scenarios pass. Manual canary: one real GraphQL query against PH returns a parsed response and a populated `Budget`.

---

- [ ] U3. **Store schema: `ph_backfill_state` table**

**Goal:** Persist backfill progress (window, cursor, last_run, last_error) in the existing SQLite store, with a proper migration from `PHTablesSchemaVersion = 1` to version 2.

**Requirements:** R3.

**Dependencies:** None (can be parallel with U1/U2).

**Files:**
- Modify: `library/marketing/producthunt/internal/store/ph_ext.go` (bump version, add table DDL and migration)
- Modify: `library/marketing/producthunt/internal/store/schema_version_test.go`
- Test: `library/marketing/producthunt/internal/store/ph_backfill_state_test.go`

**Approach:**
- Add `ph_backfill_state` table: `(window_id TEXT PRIMARY KEY, posted_after TEXT, posted_before TEXT, cursor TEXT, pages_completed INT, posts_upserted INT, last_run_at TEXT, last_error TEXT, completed_at TEXT)`.
- `window_id` is a deterministic hash of `posted_after + posted_before` so repeating the same window is idempotent.
- Bump `PHTablesSchemaVersion` from 1 to 2.
- Migration in `EnsurePHTables`: if current schema is v1, create the new table; stamp version 2 in `ph_meta`.
- Backward compatibility: a store still at v1 must migrate cleanly on the next `Open()`. Migration must be idempotent (safe to re-run).

**Execution note:** Characterization-first. Before bumping the version, add tests that the existing v1 store migrates to v2 without data loss on a real SQLite file with pre-populated v1 data.

**Patterns to follow:** Existing `EnsurePHTables` idempotent DDL pattern in `ph_ext.go`.

**Test scenarios:**
- Happy path: fresh install → `EnsurePHTables` creates all tables including `ph_backfill_state`, stamps version 2.
- Happy path: existing v1 store → migrates to v2, preserves all existing posts and snapshots.
- Edge case: `EnsurePHTables` called twice in a row → second call is a no-op, no errors.
- Edge case: store opened with a binary expecting v2 against a v1 disk → migrates; store opened with v1 binary against v2 disk → refuses to open (existing guard).
- Error path: migration SQL fails mid-way (simulate with a transaction abort) → partial state not left behind; re-run succeeds.
- Integration: `ph_backfill_state` rows survive a process restart; `SELECT` returns the same values.

**Verification:** All six scenarios pass. Manual: delete ~/.local/share/producthunt-pp-cli/data.db, run `producthunt-pp-cli doctor`, confirm new table is present.

---

- [ ] U4. **`backfill` command with pagination and budget enforcement**

**Goal:** The actual backfill command. Takes `--days N`, paginates the GraphQL `posts` query, upserts into `posts` table, persists cursor in `ph_backfill_state`, respects budget headers, supports `--dry-run`.

**Requirements:** R2, R4, R5.

**Dependencies:** U1, U2, U3.

**Files:**
- Create: `library/marketing/producthunt/internal/cli/ph_backfill.go`
- Modify: `library/marketing/producthunt/internal/cli/root.go` (register the new command in the command tree)
- Test: `library/marketing/producthunt/internal/cli/ph_backfill_test.go`

**Approach:**
- Cobra command `backfill`, flags: `--days int (default 30)`, `--from YYYY-MM-DD`, `--to YYYY-MM-DD`, `--dry-run`, `--json`.
- If `--days` given, compute window as `[now - days, now]`. If `--from`/`--to` given, use those literally. Reject combinations of `--days` with `--from`/`--to`.
- Load config, require `auth.type == "oauth"` (else print guidance to run `auth register`, exit 1).
- Open store, call `EnsurePHTables`, ensure v2.
- Compute `window_id` hash, check `ph_backfill_state` for existing row. If `completed_at` is set, print "backfill complete; use `backfill resume` only if interrupted", exit 0.
- `--dry-run`: estimate pages (window_days × ~50 posts/day ÷ 20 per page = ~75 pages for 30 days) and complexity cost (pages × 4 points). Print table; exit without calling GraphQL.
- Real run: instantiate phgraphql client, canary query of one page to calibrate actual per-page cost.
- Loop: check `client.Budget()` → if <10%, save cursor, exit 3 with rate-limit message; if 10-25%, sleep 300ms; if 25-50%, sleep 100ms; else no sleep. Execute query, parse response, upsert posts, write cursor to `ph_backfill_state`, loop.
- On `pageInfo.hasNextPage = false`: stamp `completed_at`, print summary, exit 0.
- `--json` output for agent use: single final JSON object with `{pages, posts_upserted, complexity_used, elapsed_secs, completed}`.

**Execution note:** Integration test against a mock GraphQL server first. The pagination loop and budget reaction logic are the parts most likely to be wrong the first time.

**Patterns to follow:** `internal/cli/sync.go` for the store-upsert loop shape; `internal/cli/ph_feed.go` for cobra command setup.

**Test scenarios:**
- Happy path: 30-day window, 3 pages of mock results → all posts upserted, `completed_at` stamped, exit 0.
- Happy path: `--dry-run` → prints estimate, makes zero GraphQL calls (verified via request counter on mock server).
- Edge case: `--from`/`--to` explicit window smaller than 30 days → uses those dates literally.
- Edge case: window already completed → "backfill complete" message, exit 0, no GraphQL calls.
- Edge case: `--days` combined with `--from` → rejects with usage error, exit 2.
- Edge case: 20 posts in one page, `hasNextPage = false` → single query, complete, exit 0.
- Error path: no OAuth token → "run `auth register` first", exit 1, no GraphQL calls.
- Error path: invalid token (401 from GraphQL) → "token expired, run `auth register`", exit 1.
- Error path: 429 mid-loop → save cursor, exit 3 with "resume in N minutes" message; re-running `backfill resume` picks up from saved cursor.
- Error path: budget <10% remaining → same as 429: save, exit 3, print resume hint.
- Error path: 5xx from PH → 1 retry with exponential backoff; if still failing, save cursor, exit 3.
- Integration: posts upserted by backfill are visible to `producthunt-pp-cli list --since 30d` immediately after backfill completes.
- Integration: running `backfill` then `sync` leaves no duplicate posts (upsert semantics hold across both).

**Verification:** All thirteen scenarios pass. Manual: run `producthunt-pp-cli backfill --days 30` with a real OAuth token, observe ~75 pages, <90s elapsed, ~400 complexity used, posts visible in `list --since 30d`.

---

- [ ] U5. **`backfill resume` subcommand**

**Goal:** Resume an interrupted backfill from the saved cursor.

**Requirements:** R3.

**Dependencies:** U4.

**Files:**
- Modify: `library/marketing/producthunt/internal/cli/ph_backfill.go` (add `resume` subcommand under the same parent)
- Test: extend `library/marketing/producthunt/internal/cli/ph_backfill_test.go`

**Approach:**
- `backfill resume` is a cobra subcommand of `backfill`, not a separate top-level command.
- Load `ph_backfill_state` rows where `completed_at IS NULL AND cursor IS NOT NULL`. If zero, print "nothing to resume", exit 0. If multiple, print list and require the user to pass `--window-id` to disambiguate.
- If schema version changed since the saved cursor was written (cursor format may have changed), print "store schema changed, restart backfill", exit 1.
- Else: re-enter the same pagination loop from `ph_backfill.go`, seeded with the stored cursor. Same budget enforcement.

**Execution note:** Re-use the pagination loop from U4 by factoring it into a helper that both `backfill` and `backfill resume` call. Do not duplicate the loop.

**Patterns to follow:** U4's pagination helper.

**Test scenarios:**
- Happy path: interrupted backfill (cursor saved mid-run), run `backfill resume` → picks up from saved page, completes, stamps `completed_at`.
- Edge case: no pending backfills → "nothing to resume", exit 0.
- Edge case: multiple pending backfills → prints list, requires `--window-id`.
- Edge case: `completed_at` already set → "nothing to resume", exit 0.
- Error path: saved cursor but schema version mismatch → "schema changed", exit 1.
- Error path: cursor rejected by PH (e.g., expired) → "cursor expired, restart backfill", exit 1, clear cursor.
- Integration: `backfill --days 30` interrupted by SIGTERM on page 30 → `backfill resume` completes remaining pages, total posts_upserted matches a non-interrupted run.

**Verification:** All seven scenarios pass. Manual: start a backfill, kill with SIGTERM mid-run, `backfill resume`, confirm final state is identical to a clean run.

---

- [ ] U7. **Auto-sync-on-stale-read wrapper**

**Goal:** Every read command checks if the store is stale (>24h since last `sync`) and runs sync internally before serving. No auth required. The default experience changes from "empty cold store" to "store always has the last 24h of launches."

**Requirements:** R1, R2, R3, R4, R14.

**Dependencies:** None (independent of the OAuth/GraphQL path).

**Files:**
- Create: `library/marketing/producthunt/internal/cli/ph_autosync.go` (the stale-check + sync-if-needed helper)
- Modify: `library/marketing/producthunt/internal/cli/search.go` (wire autosync in)
- Modify: `library/marketing/producthunt/internal/cli/list.go`
- Modify: `library/marketing/producthunt/internal/cli/today.go`
- Modify: `library/marketing/producthunt/internal/cli/authors.go`
- Modify: `library/marketing/producthunt/internal/cli/calendar.go`
- Modify: `library/marketing/producthunt/internal/cli/makers.go`
- Modify: `library/marketing/producthunt/internal/cli/outbound_diff.go`
- Modify: `library/marketing/producthunt/internal/cli/tagline_grep.go`
- Modify: `library/marketing/producthunt/internal/cli/trend.go`
- Modify: `library/marketing/producthunt/internal/cli/root.go` (add `--no-auto-sync` global flag and `--caller` global flag)
- Modify: `library/marketing/producthunt/internal/config/config.go` (add `auto_sync` bool field, default true)
- Modify: `library/marketing/producthunt/internal/store/ph_ext.go` (extend `ph_meta` to record `last_sync_at` if not already)
- Test: `library/marketing/producthunt/internal/cli/ph_autosync_test.go`

**Approach:**
- Single helper `EnsureFresh(store, config, flags) -> autoSyncResult` called at the top of every read command's `RunE`.
- Helper reads `ph_meta.last_sync_at`; if null or older than 24h AND auto_sync enabled AND `--no-auto-sync` not set, calls the existing `sync` internals (refactored out of `sync.go` into a reusable function).
- Records the sync attempt, elapsed time, and posts upserted; passes back to the caller so `--json` output can include `_meta.auto_synced`.
- `--caller` string is recorded in `ph_meta` alongside the sync timestamp for diagnostic purposes (not security — the field is trusted).
- Does NOT run auto-sync for write/admin commands (`sync` itself, `auth *`, `backfill *`, `doctor`, `version`, `feedback`, `api`, `workflow`, etc.).
- Auto-sync failure is not fatal: if sync errors out (network, 5xx), log the error, serve the query from whatever the store has, and include the failure in `_meta.auto_synced.error`.

**Execution note:** Characterization-first. Add a test that confirms existing read commands return the same results with and without auto-sync enabled when the store is warm. Then implement, verifying the new path only activates on stale stores.

**Patterns to follow:** `internal/cli/sync.go` for the sync internals. Extract the reusable portion into `internal/cli/ph_autosync.go` with a clean interface.

**Test scenarios:**
- Happy path: warm store (<24h), read command runs → no auto-sync triggered, `_meta.auto_synced.ran = false`.
- Happy path: stale store (>24h), read command runs → auto-sync fires, `_meta.auto_synced.ran = true` with non-zero `posts_upserted`, then query runs normally.
- Happy path: never-synced store, read command runs → auto-sync fires, identical to stale case.
- Edge case: `--no-auto-sync` flag → never fires regardless of staleness.
- Edge case: `auto_sync = false` in config → same as `--no-auto-sync`.
- Edge case: `--caller last30days/3.0.1` flag → value recorded in `ph_meta`, observable via `doctor`.
- Edge case: write/admin commands (`sync`, `auth status`, `backfill`) → auto-sync never fires for these.
- Error path: auto-sync fails (network) → error logged in `_meta.auto_synced.error`, query still runs against whatever is in the store, exit 0.
- Error path: auto-sync fails (500 from PH Atom) → same as network error.
- Integration: two reads in a row within 24h → second read sees warm store, skips sync (idempotent).
- Integration: `--json` output format — `_meta` block is present on all read commands, absent on write/admin commands.

**Verification:** All eleven scenarios pass. Manual: delete `~/.local/share/producthunt-pp-cli/data.db`, run `producthunt-pp-cli search "AI"` — should auto-sync on first call and return results, not empty.

---

- [ ] U8. **Topic-aware GraphQL enrichment (`search --enrich`)**

**Goal:** When the user runs `search <topic> --enrich` and the local store has thin results, fire a narrow GraphQL query for that topic over the last 30 days, upsert, then re-query. Opt-in per call or globally via config.

**Requirements:** R5, R6, R7.

**Dependencies:** U1, U2, U3.

**Files:**
- Modify: `library/marketing/producthunt/internal/cli/search.go` (add `--enrich`, `--enrich-threshold` flags and call path)
- Modify: `library/marketing/producthunt/internal/phgraphql/queries.go` (add topic-narrow `posts` query)
- Modify: `library/marketing/producthunt/internal/config/config.go` (add `auto_enrich` bool, default false)
- Test: extend `library/marketing/producthunt/internal/phgraphql/client_test.go` and `library/marketing/producthunt/internal/cli/search_test.go`

**Approach:**
- `--enrich` triggers enrichment when local results < `--enrich-threshold` (default 3).
- Enrichment builds a GraphQL `posts` query with `postedAfter = now - 30d`, keyword filter matching the search term (PH's `postedAfter` + post-filter in client, since PH GraphQL doesn't support fulltext directly).
- Single paginated call, ≤20 results. No loop. Budget cap per enrich: ~5 complexity points. Typical: ~1-2 points.
- Upsert results into `posts` (same path the backfill uses), then re-run the local FTS query and return.
- Emits `_meta.enrich_attempted` in `--json` output with `{attempted: true, added: N, complexity_cost: P, error: null|string}`.
- Gracefully skips (logs, continues) when: no OAuth configured, rate-limit <10% remaining, GraphQL returns error. Never fails the underlying search.
- `auto_enrich = true` in config.toml makes `--enrich` the default on every `search` without the flag.

**Execution note:** Test-first for the failure paths. The happy path is simple but the "enrichment failure doesn't fail the read" invariant is easy to get wrong without targeted tests.

**Patterns to follow:** `internal/phgraphql/client.go` for the GraphQL call; `internal/cli/search.go` for the post-FTS re-query.

**Test scenarios:**
- Happy path: `search "PostHog" --enrich` on thin store → GraphQL call fires, ≤20 posts upserted, FTS re-query returns enriched results, `_meta.enrich_attempted.added > 0`.
- Happy path: `search "PostHog"` without `--enrich` → no GraphQL call, local results only.
- Happy path: `search "PostHog" --enrich` when local already has ≥3 results → no GraphQL call (below threshold), local results returned.
- Edge case: `auto_enrich = true` in config → `search "PostHog"` enriches without needing the flag.
- Edge case: `--enrich-threshold 0` → never triggers enrichment.
- Error path: `--enrich` with no OAuth configured → enrichment silently skipped, `_meta.enrich_attempted.error = "no oauth configured"`, read returns local results.
- Error path: `--enrich` when budget <10% → enrichment silently skipped, `_meta.enrich_attempted.error = "rate limit too close"`, read returns local results.
- Error path: GraphQL returns 5xx during enrichment → enrichment silently skipped, error captured in `_meta`, read returns local results, exit 0.
- Integration: `search "PostHog" --enrich` then `search "PostHog"` (no flag) within 24h → second call finds enriched data without firing GraphQL again.
- Integration: enrichment-added posts are visible to all other commands (`list`, `trend`, etc.) because upsert goes through the same `posts` table.

**Verification:** All ten scenarios pass. Manual: `producthunt-pp-cli search "PostHog" --enrich` against a PH OAuth-authenticated store should produce actual PostHog launch records that Atom alone could not surface.

---

- [ ] U6. **Docs, SKILL.md mirror regeneration, and plugin version bump**

**Goal:** Users and agents understand the three-tier design (auto-sync default, opt-in `--enrich`, explicit `backfill`). SKILL.md, its generated mirror, and the plugin manifest all reflect the shipped behavior. Verifier passes.

**Requirements:** R15. Also completes the plugin-plumbing checklist required by `AGENTS.md`.

**Dependencies:** U1-U5, U7, U8.

**Files:**
- Modify: `library/marketing/producthunt/README.md` (rewrite around "the CLI warms itself — just run `search` and it does the right thing"; document `--enrich`, `--no-auto-sync`, `--caller`; cover `auth register` + `backfill` + `backfill resume`; clarify Atom-vs-GraphQL split and when each tier activates)
- Modify: `library/marketing/producthunt/SKILL.md` (update agent recipes: default recipe becomes "just call `search` or `list`, the CLI handles warmth"; advanced recipe documents `--enrich` for thin topics; explicit recipe documents `backfill` for first-install cold-start)
- Regenerate: `skills/pp-producthunt/SKILL.md` (run `go run ./tools/generate-skills/main.go` per AGENTS.md; this is mandatory when SKILL.md changes)
- Modify: `.claude-plugin/plugin.json` (manual semver patch bump — AGENTS.md notes the generator only auto-bumps on directory set changes, not content changes)
- Modify: `registry.json` (if the CLI's description line in the catalog needs to change; likely yes, since "Token-free" is only half the story now)
- Create: `library/marketing/producthunt/.manuscripts/20260423-000000/research/2026-04-23-000000-feat-producthunt-autowarm-brief.md` (new research note for this work; points back to the last30days feasibility study as the motivating context)
- Append: `library/marketing/producthunt/.manuscripts/20260422-231129/research/2026-04-22-231129-feat-producthunt-pp-cli-brief.md` (addendum: cold-store question posed in the original brief is answered by this plan)

**Approach:**
- README's top section should lead with "Install + run, no setup needed — the CLI auto-syncs when stale." The opt-in tiers come after.
- SKILL.md recipe hierarchy: default → `--enrich` → `backfill`. Agents should try the simpler tier first.
- Run the generator (`go run ./tools/generate-skills/main.go`) and commit the regenerated `skills/pp-producthunt/SKILL.md` alongside the library changes in one commit. AGENTS.md calls out this as "ideally one `chore(plugin): regenerate pp-* skills + bump to X.Y.Z` commit."
- Verify the skill verifier passes against the new flags: `--enrich`, `--enrich-threshold`, `--no-auto-sync`, `--caller`. Each must be declared in the appropriate `internal/cli/*.go` file and referenced correctly in SKILL.md per the verifier's rules (flag-names, flag-commands, positional-args).
- Cross-reference PR #112 retro: the README/SKILL boilerplate problem identified there must not recur. Each bullet added must be reviewed by hand against actual CLI behavior, not generator-generic language.
- Do NOT touch `spec.yaml` (still `kind: synthetic`, still declares only `/feed`). The GraphQL work lives entirely in hand-written `ph_*` files and doesn't belong in the synthetic spec.
- Do NOT touch `.printing-press.json`. Spec format hasn't changed (still synthetic/atom), just the hand-written extensions have grown.

**Patterns to follow:** Existing quickstart and recipe blocks in the producthunt README.md and SKILL.md; commit message conventions from AGENTS.md (`feat(cli)`, `chore(plugin)`, `chore(skills)`).

**Test scenarios:**
- Happy path: `.github/scripts/verify-skill/verify_skill.py` run locally against the producthunt CLI passes cleanly after docs updates.
- Edge case: every new flag mentioned in SKILL.md (`--enrich`, `--enrich-threshold`, `--no-auto-sync`, `--caller`) is declared in `internal/cli/*.go` and paired to the right command in the verifier's flag-commands check.
- Integration: the regenerated `skills/pp-producthunt/SKILL.md` is byte-equal to `library/marketing/producthunt/SKILL.md`'s content (the mirror is verbatim per AGENTS.md).
- Integration: `.claude-plugin/plugin.json` version has bumped (semver patch) from its pre-PR value.

**Verification:** All four scenarios pass. Manual: a cold reader of README.md can install, run `producthunt-pp-cli search "PostHog"`, and understand what happened behind the scenes without looking at source code. SKILL.md's default recipe, when followed by an agent, produces useful results without touching `backfill` or `auth register`.

---

## System-Wide Impact

- **Interaction graph:** Every read command (`search`, `list`, `recent`, etc.) now runs through the auto-sync wrapper (U7) before serving results. `auth register` writes to config.toml; `backfill`/`backfill resume` read config.toml + write to SQLite store. `search --enrich` (U8) adds an opportunistic GraphQL call on thin results. All three GraphQL-using paths share the same `phgraphql.Client` and same budget state.
- **Read-path latency:** reads on a fresh or stale store now incur a sync step. Cold read: 0ms → 200-500ms (one Atom fetch + parse + upsert of 50 posts). Warm read (<24h): unchanged. Integrators used to microsecond-range local reads need to budget for the first read of the day to be slower. This is called out explicitly in `_meta.auto_synced.elapsed_ms` so callers can observe it.
- **Error propagation:** `phgraphql.RateLimitedError` / `AuthError` bubble up to CLI layer, which maps them to user-friendly messages and exit codes (1 = user error, 3 = rate limit / partial success). Auto-sync failures degrade gracefully — serve local results, annotate `_meta.auto_synced.error`, exit 0. Enrichment failures do the same. Only explicit `backfill`/`auth register` failures exit non-zero.
- **State lifecycle risks:** `ph_backfill_state` is the new persistent state, plus `ph_meta.last_sync_at` / `ph_meta.last_caller` for auto-sync observability. Partial-write risk during migration is covered by SQLite transaction around `EnsurePHTables`. Cursor-out-of-date after schema bump is handled by the schema-version check in U5. Race condition: two concurrent `search` calls could both trigger auto-sync; both will succeed idempotently thanks to upsert semantics, but we may double-fetch the Atom feed. Low-priority — the Atom feed is cheap.
- **API surface parity:** Read commands add `--no-auto-sync`, `--caller`, `--json`/`_meta`. `search` adds `--enrich`, `--enrich-threshold`. Existing invocations without new flags behave identically unless the store is stale (auto-sync on) — but the sync result is the same data the user would have gotten from a manual `sync` first.
- **Integration coverage:** `backfill`, `sync`, auto-sync, and `--enrich` all target the `posts` table; upsert semantics must not double-count, lose fresh updates, or silently conflict. Tested in U4 and U8 integration scenarios.
- **Unchanged invariants:** Atom runtime stays primary on the default read path. `kind: synthetic` spec.yaml declaration is unchanged. `.printing-press.json` is unchanged. No new required env vars. `auth.type = "none"` installs continue to work — Tier 1 (auto-sync) is always-on, Tiers 2-3 silently skip when no OAuth is configured. Semver: this is a minor bump (1.0.0 → 1.1.0), not a breaking change.

---

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Complexity budget miscalibrated, backfill exceeds 50% ceiling | Canary query at run start calibrates real cost; `--dry-run` lets user preview; hard stop at 10% remaining |
| PH OAuth app registration process changes (new UI, new scopes required) | `auth register` prints the URL and prompts for creds — if the PH flow changes, user sees PH's error and can adapt. CLI doesn't hard-code the registration flow |
| PH introduces breaking GraphQL schema change | Pin to API v2 in query URL; if v3 appears, write a new plan. Versioning in request path makes breakage explicit, not silent |
| Users accidentally run `backfill` in a loop or CI | Docs block warning; `completed_at` check exits 0 immediately on already-completed windows; no auto-retry on success |
| Rate-limit header format changes | Budget math treats missing/malformed headers as "unknown, be cautious" rather than "unlimited" |
| Backfill writes partial data, then fails with a schema error | Cursor persistence is per-page in a single transaction; partial failure leaves store consistent |
| Cloudflare blocks the GraphQL endpoint (unlikely — it's api.producthunt.com, not www.) | Distinct from the www.producthunt.com CF gates; if it happens, PH sends a clear error; `backfill` surfaces that error rather than silently failing |
| PR #112-style doc boilerplate regression (README/SKILL shipped with hallucinated sections) | Explicit callout in U6 scope; each bullet reviewed against actual CLI behavior before commit |
| Auto-sync (U7) unexpectedly slows first-of-day reads | `_meta.auto_synced.elapsed_ms` in JSON output makes the cost observable; `--no-auto-sync` flag and `auto_sync = false` in config give opt-out; documented in README as expected behavior |
| Two concurrent reads both trigger auto-sync, doubling Atom traffic | Upsert semantics make the outcome identical; cheap to live with; future optimization: SQLite advisory lock around auto-sync if observed to matter |
| `--enrich` called repeatedly on the same thin topic = repeated GraphQL calls | After first successful enrichment, local store has the posts; threshold check returns "enough local results" and skips GraphQL on subsequent calls. Idempotent by construction. |
| Skill verifier fails because new flags aren't declared correctly | U6 explicitly runs the verifier as a test scenario; AGENTS.md's verifier rules documented (flag-names, flag-commands, positional-args) |
| `skills/pp-producthunt/SKILL.md` drift from library SKILL.md | U6 mandates running `go run ./tools/generate-skills/main.go` and committing both in one commit, matching AGENTS.md's "chore(plugin): regenerate pp-* skills" pattern |

---

## Documentation / Operational Notes

- After merge, Matt's memory file `feedback_pp_update_before_run.md` should be checked — users who ran the old Atom-only CLI will see new `auth register` / `backfill` subcommands; docs should make the upgrade path obvious.
- The last30days feasibility plan (`last30days-skill/docs/plans/2026-04-23-001-feat-producthunt-source-feasibility-plan.md`) should be updated with a follow-up note pointing at this plan as the answer to its "cold-store requires scheduled sync" caveat — once merged, last30days can ship the PH integration immediately rather than waiting 30 days for a warm store.
- Release notes: one line in `.manuscripts/` is sufficient. The CLI version bump (1.0.0 → 1.1.0) is the semantic signal to downstream integrators.

---

## Sources & References

- PR #112 (Atom-first runtime): https://github.com/mvanhorn/printing-press-library/pull/112
- Feasibility study that motivated this plan: `last30days-skill/docs/plans/2026-04-23-001-feat-producthunt-source-feasibility-plan.md` (cross-repo reference)
- Product Hunt API docs: https://api.producthunt.com/v2/docs
- Existing CLI: `library/marketing/producthunt/` (full source)
- Extension pattern: `internal/store/ph_ext.go`, `internal/cli/ph_feed.go`
