---
title: "fix: Broaden company-goat SEC search beyond Form D so subsidiary, debt, and acquisition signals surface"
type: fix
status: active
date: 2026-04-27
---

# fix: Broaden company-goat SEC search beyond Form D so subsidiary, debt, and acquisition signals surface

## Overview

`company-goat-pp-cli funding` returns "no Form D filings found" for companies that appear extensively in EDGAR under other form types. June Life is the canonical case: 75 EDGAR documents mention "June Life Inc." (Weber 10-K EX-21 subsidiary lists, Venture Lending & Leasing VII/VIII portfolio reports, Lucira Health and Crucible Acquisition Corp filings) but the CLI returns nothing because they raised via SAFE/notes (no Form D) and were acquired by Weber in 2020.

This plan keeps Form D as the headline killer feature and adds a fallback path: when the Form D search returns nothing, run a broader EDGAR full-text search, bin results by form type, and surface subsidiary, venture-debt, and acquisition signals. It also adds stem-variant query expansion, EDGAR 5xx retry with backoff, and actionable empty-state messages.

## Problem Frame

Reproducer from session 2026-04-27:

1. `company-goat-pp-cli snapshot --domain juneoven.com --json` returned `funding: error sec edgar 500`. EDGAR was transiently overloaded; the CLI did not retry.
2. `company-goat-pp-cli funding --domain juneoven.com` returned `no Form D filings found for "juneoven.com" (looked up by stem "juneoven")`.
3. `company-goat-pp-cli funding --domain junelife.com` returned the same after manual domain override.
4. `company-goat-pp-cli funding --who "Matt Van Horn"` and `--who "Nikhil Bhogal"` both returned `no Form D filings found naming X`.
5. The user pasted EDGAR full-text search results for `"June Life Inc."`: 75 documents over 2016-2022 across 10-K, 10-K EX-21, 10-Q, 8-K, S-1, S-1/A, DEF 14A, DRS, 424B4. Filers include Weber Inc. (post-acquisition parent), Venture Lending & Leasing VII and VIII (venture-debt holders), Lucira Health, and Crucible Acquisition Corp.

Root causes from `library/developer-tools/company-goat/internal/source/sec/sec.go` and `library/developer-tools/company-goat/internal/cli/company_funding.go`:

- `SearchFormD` always sets `q.Set("forms", "D")` (`sec.go` line 96). There is no way to run a broader EDGAR query through this client.
- `funding` and `funding-trend` use `strings.SplitN(domain, ".", 2)[0]` as the only EFTS query (`company_funding.go` lines 96, 316). EFTS indexes "June Life Inc." as a multi-token phrase; the single token `junelife` does not match it because EDGAR's analyzer splits on the camel-case boundary differently than the CLI does.
- `Client.get` returns 5xx errors immediately with no retry (`sec.go` lines 318-321). Snapshot's parallel fanout therefore drops `funding` whenever EDGAR is briefly overloaded, even though a 1-2 second backoff usually clears it.
- `runFundingWho` shares the same Form D-only path (`company_funding.go` line 136), so officer mentions in S-1, 10-K, or proxy filings never surface even though those are exactly where named officers appear most often for non-Form-D-filing companies.
- The empty-state message says "no Form D filings found" but does not point users to the broader EDGAR full-text search or suggest the `--query` / domain override that would help.

## Requirements Trace

- R1. When the Form D search returns zero results, the CLI runs a broader EDGAR search across all form types and surfaces structured mentions.
- R2. Mentions are binned by signal class so reviewers can read them at a glance: subsidiary (10-K EX-21 mentions), venture-debt (Venture Lending & Leasing 10-Q/10-K mentions), acquisition (8-K with target name), and other.
- R3. SEC EDGAR 5xx responses are retried up to 3 times with exponential backoff (200 ms, 800 ms, 2 s) before surfacing as an error. 4xx responses still return immediately.
- R4. The CLI tries multiple stem variants for the EFTS query (e.g., `junelife`, `june life`, `junelife inc`) before declaring no results, and reports which variants it tried in the empty-state message.
- R5. `funding --who` runs against all form types, not just Form D. Form D hits keep their officer/director/promoter relationship parsing; other forms surface as "named in {form} filed by {filer} on {date}".
- R6. Empty-state messages name the variants tried and suggest concrete next steps (manual EFTS URL, `--query` override).
- R7. The Form D headline positioning in `README.md` is preserved; the broader EDGAR fallback is documented as a fallback, not as a replacement.
- R8. Change ships as a PR to `main` with release notes in the description; no direct push.

## Scope Boundaries

- Not removing or repositioning Form D as the killer feature. Form D-first stays the default and the headline; broader EDGAR is a fallback path activated only when Form D is empty.
- Not parsing 10-K EX-21 subsidiary list bodies for sub-subsidiaries or full corporate-tree extraction. Subsidiary signal is detected from the EFTS hit metadata (form code + display_names), not from fetching and parsing the 10-K HTML.
- Not adding new top-level subcommands. All changes flow through existing `funding` and `funding --who` commands.
- Not changing the bare-domain UX (`company-goat-pp-cli juneoven.com` still errors with "unknown command"). User chose recommended scope, not the larger UX option.
- Not adding parallel multi-variant fanout for the stem search. Variants run sequentially with early exit on first non-empty result, to stay polite under EDGAR fair-access policy.
- Not changing the `snapshot` command's per-source contract. Snapshot benefits from R3 (retries) and R4 (variants) automatically through `secCli`, but its rendering and JSON shape stay identical.
- Not adding a new EDGAR endpoint. All work uses the existing `efts.sec.gov/LATEST/search-index` and `www.sec.gov/Archives/edgar/data/` endpoints.

### Deferred to Follow-Up Work

- Bare-domain auto-route to `snapshot` (e.g., `company-goat-pp-cli juneoven.com` → `snapshot --domain juneoven.com`): noted in the post-Phase-1.4 scope question, declined for this PR; can ship in a follow-up.
- Parallel multi-variant fanout with rate-limit-aware throttling: deferred until we see evidence the sequential-with-early-exit pattern is too slow in practice.
- 10-K EX-21 HTML parsing for full subsidiary tree: deferred. Display-name match in EFTS metadata is a strong-enough signal for v1.

---

## Context & Research

### Relevant Code and Patterns

- `library/developer-tools/company-goat/internal/source/sec/sec.go`. Single source file. `Client.SearchFormD` (line 82), `Client.FetchFormD` (line 178), `Client.SearchAndFetchAll` (line 278), `Client.get` (line 301). Shape of `SearchHit` (line 58) already includes `Form` so binning by form code is trivial once the `forms=D` filter is removed.
- `library/developer-tools/company-goat/internal/cli/company_funding.go`. `newFundingCmd` (line 41), the resolution-then-search flow (lines 86-118), `runFundingWho` (line 131), `newFundingTrendCmd` (line 286). All three hit the same `secCli.SearchAndFetchAll` chokepoint, so a single new method on `*sec.Client` lights up all three.
- `library/developer-tools/company-goat/internal/cli/company_snapshot.go`. The snapshot fanout dispatches a `funding` source with its own context and error budget; it benefits from retries automatically without changes.
- `library/developer-tools/company-goat/internal/cli/which_test.go`. Existing test file in `internal/cli`; pattern for table-driven tests against cobra commands.
- `library/developer-tools/company-goat/internal/source/sec/`. No `sec_test.go` exists yet — this PR adds the first one. Use `httptest.NewServer` to back the EDGAR endpoints.
- `docs/plans/2026-04-19-005-fix-instacart-add-notfoundbasketproduct-plan.md`. Mirror its plan structure (Problem Frame → Requirements Trace → Scope → Implementation Units with embedded helpers and dedicated test files).

### Institutional Learnings

- `docs/solutions/` does not contain a prior EDGAR retry or stem-variant solution; this is greenfield within the company-goat CLI.
- The Hacker News mentions source already implements rate-limit-aware backoff; the SEC source did not pick up the pattern when it shipped. `library/developer-tools/company-goat/internal/source/hn/hn.go` is a useful local reference for the retry shape.

### External References

- EDGAR Full-Text Search API: `https://efts.sec.gov/LATEST/search-index` accepts `forms=` as a comma-separated list (e.g., `forms=10-K,10-Q,8-K`). Omitting the parameter searches all form types.
- EDGAR fair-access policy: 10 requests/second per IP; descriptive User-Agent required. Retry-after backoff matches their published guidance.
- Form 10-K Item 21 / EX-21: "Subsidiaries of the Registrant", filed yearly by US public reporting companies. The subsidiary entity name appears in the EFTS `display_names` field of the EX-21 search hit, which is what we key on.
- Form D XML Technical Specification v9: unchanged from the existing implementation; relevant only to `FetchFormD`, not the new fallback path.

---

## Key Technical Decisions

- **Sequential variant search with early exit, not parallel fanout.** EDGAR's fair-access policy and the existing 15s HTTP timeout favor running variants one at a time and stopping on the first non-empty result. Parallel fanout would 3x our request rate per call for marginal latency gain on the case that already works.
- **Stem variants generated from the domain stem, not from a separate name resolver.** Generate `["junelife", "june life", "junelife inc"]` directly in the funding command. Avoids a new resolver hop and keeps the variant list deterministic and small.
- **Broad-EDGAR client method on `*sec.Client`, not a new package.** Add `SearchAnyForm(ctx, query, hitsPerPage int) (*SearchResponse, error)` next to `SearchFormD` so both the funding command and `--who` reuse the same retry, User-Agent, and JSON-decode plumbing. Keeps `internal/source/sec/` cohesive.
- **Mention binning happens in the CLI layer, not the SEC client.** The SEC client returns a flat `SearchHit` slice with the `Form` field populated. The funding command bins into subsidiary/debt/acquisition/other based on form type and display-name patterns. Keeps the source package agnostic of presentation logic.
- **Retry on 5xx and net errors only; 4xx returns immediately.** A 403 or 404 is intent-to-deny or genuinely-not-found; retrying just delays the user. 5xx and network errors are transient by EDGAR's own published behavior.
- **Empty-state messages name the variants tried.** The user needs to know what the CLI actually searched so they can decide whether to retry with a different query. "Tried stems: junelife, june life, junelife inc" gives them grounding.

---

## Open Questions

### Resolved During Planning

- "Should this become an EDGAR research repositioning?" Resolved: no — keep Form D as the killer feature, add broader EDGAR as a fallback. Confirmed in scope question.
- "Parallel or sequential variant search?" Resolved: sequential with early exit. Rationale above.
- "Where does mention binning live?" Resolved: in the funding command, not the SEC client.

### Deferred to Implementation

- Exact stem-variant generation rules. The simple set (`stem`, space-separated bigram if a vowel-consonant split looks like two words, `stem` + " inc") may need to grow once the test suite exercises real cases. Implementer should keep this small and add cases as tests prove they help.
- Whether the broad-EDGAR fallback should also cover `funding-trend`. Trend is fundamentally a Form D time-series signal; broadening it would dilute the metric. Leave as-is unless a test reveals user demand.

---

## High-Level Technical Design

> *This illustrates the intended approach and is directional guidance for review, not implementation specification. The implementing agent should treat it as context, not code to reproduce.*

```text
funding --domain juneoven.com
   │
   ▼
domain stem: "junelife"
variants:    ["junelife", "june life", "junelife inc"]
   │
   ▼  for each variant, sequentially (early exit on first non-empty)
SearchAndFetchAll  ──►  Form D filings found?  ──► yes ──► render headline path (existing)
   │
   ▼ no Form D found, all variants exhausted
SearchAnyForm("June Life Inc")  ──►  EDGAR EFTS, no forms filter
   │
   ▼
bin SearchHit slice by form type:
   • 10-K EX-21 mentions   →  parent_signal { parent_filer, file_date }
   • 10-K / 10-Q from
     "Venture Lending & Leasing"  →  debt_signal
   • 8-K from
     parent name              →  acquisition_signal
   • everything else         →  other_mentions
   │
   ▼
fundingResult { form_d_filings: [], yc_entry, mentions, coverage_note }
```

EDGAR 5xx retry sits one level lower, inside `Client.get`, and applies to every method on the SEC client.

---

## Implementation Units

- [ ] Unit 1: Add EDGAR 5xx retry with exponential backoff

**Goal:** Make `*sec.Client.get` retry up to 3 times on 5xx and network errors before surfacing failure, so a transient EDGAR overload no longer turns into "snapshot dropped funding entirely".

**Requirements:** R3, R8

**Dependencies:** None

**Files:**
- Modify: `library/developer-tools/company-goat/internal/source/sec/sec.go`
- Test: `library/developer-tools/company-goat/internal/source/sec/sec_test.go` (new)

**Approach:**
- Wrap the body of `Client.get` in a loop with up to 4 attempts (initial + 3 retries) and backoffs of 200 ms, 800 ms, 2 s. Use `time.Sleep` honoring `ctx.Done()`.
- Treat any `resp.StatusCode >= 500` and any `c.HTTP.Do` error (network-level failure) as retryable. 4xx returns immediately as today.
- Preserve the existing error-formatting behavior on the final attempt.
- Use `library/developer-tools/company-goat/internal/source/hn/hn.go` as a local pattern reference for backoff shape.

**Patterns to follow:**
- The HN source's existing rate-limit-aware backoff
- Standard library `context` cancellation handling (do not retry after `ctx.Err() != nil`)

**Test scenarios:**
- Happy path: server returns 200 on first attempt; one HTTP call recorded.
- Transient: server returns 500 on first attempt, 200 on second; two calls recorded; result equals second body.
- Persistent server error: server returns 500 on every attempt; final error after 4 calls; error message includes the 500 status.
- Permanent client error: server returns 403; one call recorded; no retry; error surfaced immediately.
- Context cancellation: caller cancels context during the first backoff; loop exits with `ctx.Err()`; no further calls.
- Network failure: `httptest` server closed before request; net error caught and retried up to the cap; final error preserved.

**Verification:**
- A snapshot run that hits a transient EDGAR 500 returns funding data on the retry rather than dropping it.

---

- [ ] Unit 2: Add `SearchAnyForm` to the SEC client

**Goal:** Provide a single `*sec.Client` method that runs an EFTS full-text search across all form types, so the funding command can fall back to broader EDGAR mentions when Form D is empty.

**Requirements:** R1, R2

**Dependencies:** Unit 1 (so the new method inherits retry behavior automatically; not strictly blocking but lands in the same package)

**Files:**
- Modify: `library/developer-tools/company-goat/internal/source/sec/sec.go`
- Test: `library/developer-tools/company-goat/internal/source/sec/sec_test.go`

**Approach:**
- Add `SearchAnyForm(ctx context.Context, query string, hitsPerPage int) (*SearchResponse, error)` next to `SearchFormD`.
- Reuse the existing EFTS request building, but omit `q.Set("forms", "D")`. Everything else (User-Agent, decode, hit shape) stays identical.
- Reuse the existing `SearchHit` and `SearchResponse` types so callers get the `Form` field populated for binning.
- Add a small private helper `searchEFTS(ctx, query, formsFilter, hitsPerPage)` that both `SearchFormD` and `SearchAnyForm` call, to avoid copy-paste of the request-building logic.

**Patterns to follow:**
- Existing `SearchFormD` JSON decoding shape
- Existing `Client.get` response handling

**Test scenarios:**
- Happy path: query returns mixed 10-K, 8-K, S-1 hits; response includes a `Form` value on each hit; the SearchResponse total matches the EFTS envelope.
- Empty result: query returns zero hits; SearchResponse is non-nil with empty Hits and Total = 0; no error.
- Refactor parity: a query that previously worked through `SearchFormD` still returns the same Form D hits when called through `searchEFTS(ctx, query, "D", n)`. Guards against regression in the shared helper.
- Decode failure: malformed JSON body; error mentions "decode efts response".

**Verification:**
- `funding` and `funding --who` paths can call `SearchAnyForm` and receive hits with `Form` values populated for binning.

---

- [ ] Unit 3: Generate stem variants and try them sequentially in `funding`

**Goal:** When the Form D search returns nothing for the primary stem, retry with a small set of name variants before declaring failure; carry the variant list forward so empty-state messages can name what was tried.

**Requirements:** R4, R6

**Dependencies:** None

**Files:**
- Modify: `library/developer-tools/company-goat/internal/cli/company_funding.go`
- Test: `library/developer-tools/company-goat/internal/cli/company_funding_variants_test.go` (new)

**Approach:**
- Add a private `stemVariants(domain string) []string` helper that, given `junelife.com`, returns `["junelife", "june life", "junelife inc"]`. Keep the rules small and documented inline:
  - Always include the bare stem.
  - Add a space-split bigram only when the stem looks like two concatenated words (vowel-consonant boundary heuristic; if uncertain, skip).
  - Add a `<stem> inc` variant for plausible US private-company legal-name fallback.
- In the `funding` `RunE`, replace the single `secCli.SearchAndFetchAll(ctx, stem, maxFilings)` call with a loop over `stemVariants(domain)`. Stop on the first variant that returns at least one filing.
- Track which variants were attempted; carry the slice forward as a struct field for use by the empty-state path in Unit 5.
- Variants run sequentially (no parallel fanout) per the EDGAR fair-access policy and the Key Technical Decisions section.

**Patterns to follow:**
- Existing `strings.SplitN(domain, ".", 2)[0]` stem extraction
- Cobra command flag handling already in `newFundingCmd`

**Test scenarios:**
- Single-token domain: `stripe.com` → `["stripe", "stripe inc"]`. No bigram.
- Concatenated domain: `junelife.com` → `["junelife", "june life", "junelife inc"]`. Bigram included.
- Hyphenated domain: `acme-corp.com` → `["acme-corp", "acme corp", "acme-corp inc"]`. Hyphen treated as a word boundary.
- Numeric domain: `404.com` → `["404", "404 inc"]`. No bigram (no clean vowel-consonant split).
- Early exit: first variant returns 1 filing; only 1 SearchAndFetchAll call recorded.
- All exhausted: all variants return 0 filings; 3 calls recorded; empty-state path receives the full variant list.

**Verification:**
- For `junelife.com`, the search loop tries the bigram variant before giving up. The empty-state JSON output includes `"stems_tried": ["junelife", "june life", "junelife inc"]`.

---

- [ ] Unit 4: Wire broad-EDGAR fallback into `funding` with mention binning

**Goal:** When all stem variants return zero Form D filings, run a single broad-EDGAR search and return binned mentions (subsidiary, debt, acquisition, other) so the user sees the available signal instead of "nothing found".

**Requirements:** R1, R2, R7

**Dependencies:** Unit 2 (needs `SearchAnyForm`), Unit 3 (needs variant list to choose the broad-search query)

**Files:**
- Modify: `library/developer-tools/company-goat/internal/cli/company_funding.go`
- Test: `library/developer-tools/company-goat/internal/cli/company_funding_mentions_test.go` (new)

**Approach:**
- Extend `fundingResult` with a new field `Mentions *fundingMentions` (omitempty). Type `fundingMentions` carries `Subsidiary []mentionRow`, `Debt []mentionRow`, `Acquisition []mentionRow`, `Other []mentionRow`, plus a `Total int` count. `mentionRow` carries `Form`, `Filer`, `FileDate`, `AccessionURL`.
- After Form D exhaustion (Unit 3 returns empty across variants), call `secCli.SearchAnyForm(ctx, primaryQuery, 25)` once with the most distinctive variant (the bigram if present, else the bare stem, else `<stem> inc`).
- Bin results by simple form-and-filer rules:
  - `Form == "10-K"` and `display_names` contains the subject company → `Subsidiary` (the parent is the filer, the subject is in the EX-21 list).
  - Filer name starts with `"Venture Lending & Leasing"` (any roman-numeral suffix) → `Debt`.
  - `Form == "8-K"` and `display_names` contains the subject company → `Acquisition` candidate (announcement filings; user can drill into the linked accession).
  - Anything else → `Other`.
- Build `AccessionURL` as `https://www.sec.gov/Archives/edgar/data/<cik-int>/<dashless-accession>/` so users can click through.
- Update the "no Form D" branch to render mentions when present and only fall through to the empty-state message when both Form D and broad-EDGAR are empty.
- Update `coverage_note` text: when mentions found, replace the generic "Form D is US-only..." with "No Form D filings; surfaced N EDGAR mentions across other filings (Form D coverage note still applies)."

**Patterns to follow:**
- Existing `fundingFilingsFromSEC` projection helper
- Existing `renderFunding` text vs JSON branching

**Test scenarios:**
- June Life canonical: stub `SearchAnyForm` to return 4 hits — 1 Weber 10-K EX-21 mention, 2 Venture Lending & Leasing 10-Q mentions, 1 Weber 8-K mention. Result has `Subsidiary` count 1 with `filer` "Weber Inc.", `Debt` count 2, `Acquisition` count 1, `Other` count 0.
- All-other classifier: stub returns hits from neither Weber nor VL&L (e.g., a generic SEC filer mentioning the name in passing); all hits land in `Other`.
- Empty broad search: both Form D and broad search return empty; result has no `mentions` key in JSON; render path falls through to the existing exit-5 empty-state.
- Coverage note swap: when mentions present, `coverage_note` matches the new text and not the old "Form D is US-only..." string.
- AccessionURL shape: a hit with CIK `0001890586` and accession `0001628280-21-024546` produces `https://www.sec.gov/Archives/edgar/data/1890586/000162828021024546/`.
- Integration: end-to-end test through the cobra command with a `httptest.NewServer` backing both EFTS endpoints; asserts JSON output structure for the June Life shape.

**Verification:**
- Running `funding --domain junelife.com --json` against a test server returning the canonical shape produces a JSON document with `form_d_filings: []` and a populated `mentions.subsidiary[0].filer = "Weber Inc."`.

---

- [ ] Unit 5: Broaden `funding --who` to all forms

**Goal:** Person-named searches surface mentions across S-1, DEF 14A, 10-K, and 8-K filings, not just Form D, so officers and named executives of non-Form-D-filing companies appear.

**Requirements:** R5, R6

**Dependencies:** Unit 2 (needs `SearchAnyForm`)

**Files:**
- Modify: `library/developer-tools/company-goat/internal/cli/company_funding.go`
- Test: `library/developer-tools/company-goat/internal/cli/company_funding_who_test.go` (new; existing tests for runFundingWho live mixed in `which_test.go` if any)

**Approach:**
- In `runFundingWho`, run two passes:
  1. Existing path: `SearchAndFetchAll` (Form D), then filter by parsed `RelatedPersons` (officer/director/promoter relationship match). Output unchanged for these hits.
  2. New path: `SearchAnyForm` for the same person query, filter to hits where any `display_names` entry contains the person's name (case-insensitive). Bin by form type.
- Combine outputs into a single result with two sections: `form_d_filings` (parsed and high-confidence) and `mentions` (form-type-binned, lower-confidence). Both use the same `mentionRow` shape from Unit 4.
- Update help text on `--who` to reflect broader scope: "Show every Form D filing where the named person is a related party, plus other EDGAR mentions of the name across S-1, 10-K, DEF 14A, etc."
- Maintain exit code 5 only when both paths return empty.

**Patterns to follow:**
- Existing `runFundingWho` filter loop on `RelatedPersons`
- Mention binning helper from Unit 4

**Test scenarios:**
- Form D only: stub returns Form D hits; mentions array empty; output preserves existing JSON shape with `form_d_filings` populated.
- Mentions only: stub returns no Form D hits but 3 S-1 / DEF 14A hits naming the person; `form_d_filings` empty; `mentions.other` has 3 entries.
- Both: stub returns 1 Form D hit and 2 mention hits; both arrays populated; ordering preserved.
- Both empty: stub returns nothing on either path; exit code 5; empty-state message names the variants tried (here just the person name) and points to `https://efts.sec.gov/LATEST/search-index?q=<urlencoded>`.
- Case-insensitive name match: stub display_names contains `"Patrick Collison"`; query `"patrick collison"` matches.
- No false positive on substring-only match: stub display_names contains `"Patrick Collinsworth"`; query `"Patrick Collins"` does not match (word-boundary logic, not raw substring).

**Verification:**
- `funding --who "Mike Tudor" --json` (a June Life co-founder, not a Form D officer in any current filing) returns at least one entry under `mentions` if EDGAR has any 8-K or 10-K naming the person.

---

- [ ] Unit 6: Empty-state messages, README/SKILL doc updates, and PR

**Goal:** When everything genuinely returns zero, the empty-state message tells the user what was searched, what variants were tried, and what to try next; surface the broader EDGAR fallback in user-facing docs without unseating the Form D headline.

**Requirements:** R6, R7, R8

**Dependencies:** Unit 4, Unit 5

**Files:**
- Modify: `library/developer-tools/company-goat/internal/cli/company_funding.go`
- Modify: `library/developer-tools/company-goat/README.md`
- Modify: `library/developer-tools/company-goat/SKILL.md`

**Approach:**
- Update both empty-state branches (`funding` and `funding --who`) to print:
  - The variants/queries actually tried.
  - A direct EFTS URL the user can paste into a browser.
  - A suggestion to run `--query` with an exact issuer name.
  - Coverage caveat (Form D is US-only; SAFE rounds not covered).
- README.md: add a "When Form D is empty" subsection under the killer-feature description, showing the new mentions binning with a short June Life-style example. Do not move Form D out of the headline.
- SKILL.md: agent-facing doc gets one paragraph noting that `funding` now also surfaces subsidiary, debt, and acquisition signals from broader EDGAR mentions when Form D is empty, and that `funding --who` searches all form types.
- Open a PR to `mvanhorn/printing-press-library` with title `fix(company-goat): broaden SEC search beyond Form D` and a description that links the June Life session reproducer.

**Patterns to follow:**
- Existing README.md tone (terse, example-led)
- Existing SKILL.md format (sections under "Highlights" / "Coverage")

**Test scenarios:**
- Test expectation: none — this unit is documentation and empty-state copy refinements with no behavioral change beyond what Units 1-5 deliver. Manual review of rendered text and the published README is sufficient.

**Verification:**
- The PR description includes the June Life reproducer and at least one screenshot or pasted JSON of the new mentions output.
- `funding --domain made-up-shell-company.example` prints a message naming the variants tried and links to the EFTS browser URL.

---

## System-Wide Impact

- **Interaction graph:** `secCli` is shared by `funding`, `funding-trend`, `funding --who`, and the `snapshot` fanout. Unit 1 (retries) lifts every caller automatically. Unit 2's new `SearchAnyForm` is opt-in; only the funding command and `--who` invoke it. `snapshot`'s funding source benefits from R3 retries but its rendering is unchanged.
- **Error propagation:** Retries happen inside `Client.get`. Surface-level error contracts (the strings `funding` and `--who` print) widen only in the empty-state paths, which previously printed terse "no Form D" messages and now print variant-aware guidance.
- **State lifecycle risks:** Stem-variant loops are sequential and idempotent. EDGAR responses are not cached locally, so re-running is safe. No partial-write concerns.
- **API surface parity:** `funding-trend` is intentionally not broadened (Key Technical Decisions). Its behavior is unchanged except that it inherits Unit 1 retries.
- **Integration coverage:** Unit 4's integration test through `httptest.NewServer` exercises the full Form-D-empty → broad-EDGAR-search → mention-binning chain end to end. This is the minimum integration coverage that a unit-test-only mock would not prove.
- **Unchanged invariants:** `fundingResult` keeps `form_d_filings` as its primary field with the same shape. The new `mentions` field is `omitempty`, so the existing JSON shape is preserved for current Form-D-rich responses (e.g., `funding stripe`).

---

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Stem-variant heuristic generates noisy variants for some domains, surfacing irrelevant 10-K mentions | Variants run sequentially with early exit on first non-empty; broad-EDGAR fallback runs only when all Form D variants are empty, which already filters to genuinely-rare cases. Test scenarios in Unit 3 cover degenerate domain shapes. |
| Subsidiary detection produces false positives when display_names contains a name substring (e.g., "June" inside "Junebug Inc.") | Match against the full bigram or quoted phrase, not the bare stem. Test scenario in Unit 4 explicitly covers the substring-not-match case. |
| EDGAR raises the fair-access bar and starts blocking the new fallback path | Sequential variants + early exit + 25-hit cap on broad search keeps request rate well below 10/sec. User-Agent already includes contact email. If blocked, retry path surfaces a clear error. |
| Mention binning misses a class of filer pattern not in the test set | Unit 4's `Other` bucket is the catch-all; nothing is dropped silently. Future tests can promote new patterns into `Subsidiary` / `Debt` / `Acquisition` without breaking existing output. |
| README repositioning accidentally weakens the Form D headline | Unit 6 adds the broader EDGAR section as a subsection under the killer-feature description, not as a peer. Reviewer should explicitly check headline framing. |

---

## Documentation / Operational Notes

- README.md and SKILL.md updates ship with the PR (Unit 6).
- No release-notes infrastructure beyond the PR description; the printing-press-library publishes via tag-driven release.
- No new environment variables. `COMPANY_PP_CONTACT_EMAIL` continues to be the only SEC-related env var.
- Polish-worker scorecard delta expected: small positive for `verify` (no regression) and `agent_native` (better empty-state guidance).

---

## Sources & References

- Session reproducer: 2026-04-27 conversation in this Claude Code session, including the user's pasted EDGAR full-text search results for `"June Life Inc."` (75 documents).
- Source files: `library/developer-tools/company-goat/internal/source/sec/sec.go`, `library/developer-tools/company-goat/internal/cli/company_funding.go`, `library/developer-tools/company-goat/internal/cli/company_snapshot.go`.
- Plan structure pattern: `docs/plans/2026-04-19-005-fix-instacart-add-notfoundbasketproduct-plan.md`.
- EDGAR Full-Text Search: `https://efts.sec.gov/LATEST/search-index`.
- EDGAR fair-access policy: `https://www.sec.gov/os/accessing-edgar-data`.
- Form D XML Technical Specification v9 (unchanged from existing implementation).
