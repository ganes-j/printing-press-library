# company-pp-cli Brief

## API Identity
- **Domain:** Synthetic multi-source CLI — no single API. Blends 7 free sources to deliver company research focused on small/midsize startups.
- **Users:** Scouts and small-check investors (sub-$2M tickets), founders sizing up competitors / partners / acquirers, BD/sales prospecting, job seekers evaluating startup growth, LLM agents that need structured company-data tools.
- **Data profile:** Per-company snapshot blending fundraising signals (Form D), engineering activity (GitHub), launch/discussion signals (HN), legal entity (Companies House UK + SEC EDGAR US), reference (YC, Wikidata), and domain age (WHOIS/DNS). All free, all public, all real APIs.

## Reachability Risk
- **Low.** All 7 sources are well-documented, no aggressive bot-detection, stable rate limits.
- One caveat: SEC EDGAR requires a descriptive `User-Agent` header per fair-access policy. Will set `User-Agent: company-pp-cli (<contact-email>)` from a config-overridable default.
- HN Algolia and YC's static JSON snapshot have no rate limits in practice. Wikidata SPARQL allows ~5 req/sec.

## Source Priority
Confirmed via Multi-Source Priority Gate (`source-priority.json`). Fanout-style CLI; "primary" affects README hero and headline command, not query order.

| Rank | Source | Spec state | Auth |
|------|--------|------------|------|
| 1 | **SEC EDGAR Form D** (primary, killer feature) | Documented public APIs — `data.sec.gov` (RESTful JSON, no key) + `efts.sec.gov` (full-text search) | None (User-Agent required) |
| 2 | GitHub | OpenAPI spec via `gh` SDK | Optional `GITHUB_TOKEN` (60 → 5000 req/hr) |
| 3 | Hacker News (Algolia) | Public API at `hn.algolia.com/api/v1/` | None |
| 4 | Companies House (UK) | REST API at `api.company-information.service.gov.uk` | **Optional** `COMPANIES_HOUSE_API_KEY` — without it, `legal --uk` returns setup instructions |
| 5 | YC directory | Static JSON snapshot via `yc-oss.github.io/api/` (updated daily from YC's Algolia index) | None |
| 6 | Wikidata | SPARQL at `query.wikidata.org/sparql` | None |
| 7 | WHOIS / DNS | RDAP (`rdap.org`) + DNS via Go stdlib | None |

**Economics:** Primary (Form D) is free and key-less. Companies House is the only keyed source and is intentionally optional — `legal <co>` falls back to SEC EDGAR Form D issuer data for US entities, and prints a setup hint for UK lookup when no Companies House key is present. No paid sources bleed into primary commands.

**Inversion risk:** None. This is a fanout combo CLI; spec completeness across sources doesn't invert priority because every source has a clean documented API.

**Dropped:** OpenCorporates — shifted to paid tiers; not realistic for a free CLI. US legal entity is recovered indirectly via SEC EDGAR Form D issuer name + state of incorporation.

## Top Workflows
1. **Quick snapshot of a company** — `company-pp-cli snapshot stripe` — fanout across all 7 sources, render unified per-source summary in seconds. The headline workflow.
2. **Did this startup actually raise?** — `company-pp-cli funding anthropic` — Form D filings show offering amount, date, exemption claimed, related entities. The killer-feature command.
3. **Is this startup engineering-heavy?** — `company-pp-cli engineering vercel` — GitHub org size, repo count, contributor count, commit cadence as a "is this team building" signal.
4. **Is this UK company legitimate?** — `company-pp-cli legal --uk monzo-bank` — Companies House profile + officers + filing history.
5. **What's the launch story?** — `company-pp-cli launches replit` — Show HN posts ordered by points, with year hints to spot dead vs. active launches.
6. **Compare two companies side-by-side** — `company-pp-cli compare ramp brex` — two snapshots aligned in columns.

## Table Stakes (matched from Crunchbase/OpenVC/PitchBook web UIs)
- Company profile (name, website, description, location)
- Founding date / founders
- Funding rounds (amount, investors, date)
- Recent news mentions
- Tech stack signals (engineering team size as proxy)
- Launch/discussion timeline
- Legal entity verification
- Side-by-side compare

The free combo we're building matches all of these for the **small/midsize startup tier** (where Crunchbase coverage is thinnest and most valuable). Form D in particular is a real differentiator — Crunchbase Free shows trivial fundraising data; Form D shows the underlying SEC filings with structured offering details.

## Data Layer
- **Primary entities:** `companies` (resolved by domain), `form_d_filings`, `github_orgs`, `hn_mentions`, `yc_entries`, `wikidata_entities`, `companies_house_records`, `domain_records`.
- **Sync cursor:** Per-company timestamp of last sync. `sync <co>` populates the local SQLite store; subsequent reads can use `--data-source local|live|auto` (auto refreshes if older than configurable threshold).
- **Resolution cache:** Name → domain mappings are cached after first successful resolution to avoid re-querying Wikidata/YC/DNS for repeat queries on the same name.
- **FTS5/search:** Full-text search across resolved company set so users can `search "fintech b2b 2024"` and get matching companies from their local sync history.

## Codebase Intelligence
- **No single SDK exists for this combination.** All sources have official docs but no aggregator wraps them.
- Closest community projects:
  - `yc-oss/api` — daily-updated YC company JSON snapshots; we'll use this for the `yc <co>` command.
  - `janlukasschroeder/sec-api-python` — Python SDK for EDGAR; useful as a reference for Form D XML parsing patterns but won't be a runtime dependency.
- **Form D XML structure** (Form D XML Technical Specification v9):
  - `primaryIssuer`: `cik`, `entityName`, `entityType`, `issuerAddress` (street1, city, stateOrCountry, zipCode), phone
  - `offeringData.industryGroup.industryGroupType`: e.g., "Pooled Investment Fund", "Technology", "Biotech"
  - `offeringData.totalOfferingAmount`, `totalAmountSold`
  - `offeringData.exemptionClaimed`: `06b` (Reg D 506(b)), `06c` (506(c))
  - `relatedPersons`: officers and major holders
- **Form D filing URLs:** `https://www.sec.gov/Archives/edgar/data/<CIK>/<accession-no-dashes>/primary_doc.xml` after locating accession via full-text search at `efts.sec.gov/LATEST/search-index?q=<term>&forms=D`.
- **Auth across sources:**
  - GitHub: `Authorization: Bearer <token>` (env: `GITHUB_TOKEN`, falls back to `gh auth token`)
  - Companies House: HTTP Basic with key as username and blank password (env: `COMPANIES_HOUSE_API_KEY`)
  - SEC EDGAR: `User-Agent: company-pp-cli (<contact>)` only; no key
  - Others: no auth

## User Vision
The user explicitly framed this as a **goat-style multi-source CLI** built around the Form D unlock. Their stated framing during planning:
- "The killer feature is SEC EDGAR Form D unlock — most US startups raising priced rounds file Form D and almost nobody outside finance knows this is queryable for free. That alone replaces a chunk of what people pay Crunchbase/PitchBook for."
- Architecture must be **self-contained** — do not shell out to other printed CLIs even when they exist (e.g., `pp-producthunt`).
- **Honesty contract** is non-negotiable: absent signals must render explicitly ("no Form D filings found", "no GitHub org found"), never silently omitted.
- Disambiguation when name → multiple candidates: numbered list + exit code 2 + JSON mode for agent consumption.
- Explicit non-goals: not a Crunchbase replacement (no global private-market funding graph), not a tech-stack profiler (deferred to v2), no composite "alive-ness score" (cut as snake oil).

## Product Thesis
- **Name:** `company-pp-cli` (skill: `pp-company-goat`)
- **Display name:** "Company Goat"
- **Headline:** "Look up startups across SEC Form D, GitHub, Hacker News, Companies House, YC, and Wikidata in one command — including the SEC fundraising data Crunchbase Free hides behind a paywall."
- **Why it should exist:**
  - Crunchbase free tier is gone; paid is thousands/month. Form D alone gives back a real chunk of what Crunchbase Pro shows for US startups.
  - No existing CLI fans across these sources. Today this is 8 browser tabs and 30 minutes per company.
  - Agent-native: structured output with `--json`, disambiguation with `--pick`, explicit absence rendering — exactly what an LLM agent needs to research a company without prose summarization tax.
  - Bonus: works offline after `sync`, lets you build local datasets you can SQL-query.

## Build Priorities

### Priority 0 — Foundation
1. SQLite store with company-keyed schema (resolved by domain).
2. Name resolution module: parallel Wikidata + YC + domain probe → candidate list with disambiguation.
3. Per-source HTTP clients with appropriate auth headers and rate limiting.
4. `cliutil.FanoutRun` integration for parallel source queries with per-source error capture and source-order output.

### Priority 1 — Per-source commands (absorbed/table-stakes)
1. `funding <co>` — SEC EDGAR Form D + YC batch lookup (the killer feature)
2. `engineering <co>` — GitHub org/repos/contributors/commits
3. `launches <co>` — Show HN posts via Algolia search filtered to `_tags:show_hn`
4. `mentions <co>` — HN mention timeline via Algolia full-text search with date histogram
5. `legal <co>` — Companies House (UK, with optional key) + SEC EDGAR Form D issuer fields (US)
6. `yc <co>` — YC directory entry from yc-oss snapshot
7. `wiki <co>` — Wikidata SPARQL company facts
8. `domain <co>` — RDAP/WHOIS + DNS CNAME → hosting hint (Vercel/Netlify/Heroku/Cloudflare Pages/AWS/GCP)
9. `sync <co>` — Populate local store (calls all source clients, persists)

### Priority 2 — Transcendence (only possible because of local data)
10. `snapshot <co>` — Fanout summary (P1 commands run in parallel via cliutil.FanoutRun)
11. `compare <a> <b>` — Two snapshots side-by-side
12. `search <query>` — FTS5 search across local synced company set
13. `funding-trend <co>` — Form D filings over time as a chart-friendly time series
14. `signal <co>` — Cross-source consistency check: "Form D says raised $5M in 2023, but GitHub org has 0 commits since 2022 — possible zombie."

These transcendence commands are unique to this CLI's local-data approach: no source individually answers them; only the local SQLite store makes the joins possible.

## Reachability Probe
Will run a single GET against `https://data.sec.gov/submissions/CIK0001318605.json` (Tesla, well-known CIK) with proper User-Agent before generation to confirm SEC EDGAR is reachable from this machine.
