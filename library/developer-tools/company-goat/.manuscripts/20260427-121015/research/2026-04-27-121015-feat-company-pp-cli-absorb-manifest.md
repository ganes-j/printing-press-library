# company-pp-cli Absorb Manifest

## Competitive landscape

This is a **niche-empty** CLI category. No existing terminal tool aggregates company-research data across these sources. Closest analogs:

| Tool | Type | Coverage | Why it doesn't compete |
|------|------|----------|-----------------------|
| Crunchbase web UI | Web | Funding, profile, news, similar | Free tier shows trivial data; Pro is $99/mo personal, thousands/mo enterprise. No CLI. |
| OpenVC | Web | Investor directory + light company data | Investor-focused, not company-focused. Has free public API beta. |
| Owler | Web | Profile, news, competitors | Free tier limited; paid; web only. |
| PitchBook | Web | Comprehensive private/public data | Enterprise paywall, $thousands/year, no CLI. |
| CB Insights | Web | AI-powered private company intel | Enterprise paywall. |
| Tracxn | Web | Asia-focused | Paywall. |
| `gh` CLI | CLI | GitHub only | Single-source; we wrap it for engineering signal. |
| `edgartools` (Python) | Python lib | All SEC forms | Library not CLI; Python; not company-focused. |
| `sec-api-python` | Python lib | All SEC data via paid API | Paid service. |
| `yc-oss/api` | JSON snapshot | YC directory only | Static data feed; we consume. |

**The gap:** No tool fans out across SEC + GitHub + HN + Companies House + YC + Wikidata. Today this is 8 browser tabs and 30 minutes per company.

## Step 1.5a/b: Absorbed features

For a synthetic combo CLI, "absorption" means matching the value proposition of competing **web** products (since no CLI competitor exists). Every absorbed row gets `--json`, `--dry-run` (where applicable), typed exit codes, and SQLite persistence.

| # | Feature | Best Source / Inspiration | Our Implementation | Added Value |
|---|---------|--------------------------|--------------------|-------------|
| 1 | Company profile snapshot | Crunchbase web profile page | `snapshot <co>` — fanout across 7 sources | Multi-source, agent-native, offline after sync |
| 2 | Funding rounds & Form D | Crunchbase Pro / SEC EDGAR direct | `funding <co>` — direct EDGAR Form D extraction | Free, structured offering amounts, exemption type, investor names |
| 3 | Company name → domain resolution | Crunchbase autocomplete | `resolve <name>` with disambiguation list | Multi-source (Wikidata + YC + DNS), cached in store |
| 4 | UK legal entity lookup | Companies House web search | `legal --uk <co>` — REST API client | --json, status filters, officers list |
| 5 | US legal entity surface | Crunchbase Pro | `legal --us <co>` — Form D issuer fields | Free, includes state of incorporation, entity type |
| 6 | Engineering org signal | (no clean web parallel) | `engineering <co>` — GitHub org/repos/contributors | New signal: "is the team actually building?" |
| 7 | Launch story (Show HN) | (manual HN search) | `launches <co>` — Algolia filtered Show HN | Sorted by points + year for relevance |
| 8 | Mention timeline | (manual HN search) | `mentions <co>` — Algolia full-text histogram | Time-bucketed mentions over years |
| 9 | YC backing status | YC directory web search | `yc <co>` — yc-oss snapshot | Batch, status, location in one shot |
| 10 | Reference facts | Wikipedia infobox | `wiki <co>` — Wikidata SPARQL | Structured: founded, HQ, founders, industry |
| 11 | Domain age & hosting | (manual whois + dig) | `domain <co>` — RDAP + DNS CNAME match | Hosting hint (Vercel/Netlify/etc.) included |
| 12 | Side-by-side compare | Crunchbase compare view (Pro) | `compare <a> <b>` — two snapshots aligned | Free, renders in terminal |
| 13 | Local sync + offline query | (n/a, web tools are online) | `sync <co>` + `--data-source local` | Build research dataset; SQL-queryable |

## Step 1.5c: Transcendence features (only possible with our local-data approach)

Per the skill's user-first feature discovery framework: who are our users, what are their rituals, and what questions can't they answer today?

**Persona 1: Solo angel scout** — gets ~10 inbound deals/week, screens each in <5 min, mostly looking for "is this real and growing." Today: tabs Crunchbase Free, opens GitHub org if engineering-heavy, glances at LinkedIn for team size, gives up on Form D because it's painful. Wants a one-shot snapshot.

**Persona 2: Founder evaluating partnerships** — "should I integrate with this startup?" Wants to know: are they funded, are they shipping, are they responsive on HN, are they likely to be around in 12 months.

**Persona 3: Job seeker evaluating offers** — "this startup pitched me a senior role at 0.4% — is this worth it?" Wants: real funding (not 'raised undisclosed'), engineering momentum (commits accelerating or decelerating), and any zombie-company red flags.

**Persona 4: BD/sales prospecting** — "build a list of 50 fintechs in YC W22-W24 with active HN presence." Wants bulk querying after one-time sync.

| # | Feature | Command | Score | Why Only We Can Do This |
|---|---------|---------|-------|------------------------|
| 1 | **Cross-source signal check** | `signal <co>` | 9/10 | Compares "Form D says raised $5M in 2024" with "GitHub org has 0 commits since 2022" — no single source can do this. Surfaces zombie companies, stale fundraising, suspicious mismatches. |
| 2 | **Funding trend over time** | `funding-trend <co>` | 8/10 | Time-series of Form D filings showing fundraising cadence. Useful for charting and detecting "didn't raise in 2024" silently. Form D paginated by date is hard via web UI. |
| 3 | **Local FTS search across synced companies** | `search <query>` | 8/10 | After syncing N companies, FTS5 finds matches by description, founders, location, industry — across the local set. Personal research database that compounds. |
| 4 | **Snapshot fanout** | `snapshot <co>` | 9/10 | The headline command. Web products give partial info per page; we give all 7 sources in one terminal-rendered view in seconds. |
| 5 | **Side-by-side compare** | `compare <a> <b>` | 7/10 | Web tools require Pro tier and are visual-only; we do this for free, free-form, and pipe-friendly. |
| 6 | **Bulk export of synced set** | `sync --batch <names...>` then read store | 6/10 | After bulk sync, export companies as JSON/CSV for further analysis. The local store IS the deliverable for power users. |

All 6 score >= 5/10. Forming the `novel_features` array.

## Step 1.5c.5: Auto-suggest novel features (additional brainstorming)

User vision (from briefing): The user explicitly stated this is built around **the Form D unlock**. Their framing: "Most US startups raising priced rounds file Form D and almost nobody outside finance knows this is queryable for free."

Additional brainstorm aimed at the killer feature:
- **Form D investor cross-reference** — Form D filings list "related persons" (officers, advisors, large holders). Build a graph: "What other companies did <person X> file Form D for?" Answers: who's prolific in this space?

Adding:

| # | Feature | Command | Score | Why |
|---|---------|---------|-------|-----|
| 7 | **Form D related-person graph** | `funding --who <name>` | 7/10 | "Show me all Form D filings where this person is named." Reveals serial founders / repeat investors. Pure Form D data, no other source. |

## Reprint reconciliation

Not applicable — first generation.

## Stub features

None. Every feature in this manifest is implementable against real, free APIs we've already probed for reachability. No stubs in v1.

## Final summary

- **13 absorbed features** (matched competing web products with --json + offline + agent-native)
- **7 transcendence features** (only possible because of multi-source + local store)
- **20 features total** in shipping scope
- **0 stubs**

For comparison, Crunchbase Pro (paid) shows ~12 sections per company; we cover or improve on most of them with a CLI surface and add 7 features none of them offer.
