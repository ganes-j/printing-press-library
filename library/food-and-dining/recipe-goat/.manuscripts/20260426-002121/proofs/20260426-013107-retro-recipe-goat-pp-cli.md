# Printing Press Retro: recipe-goat (regenerate)

## Session Stats

- API: recipe-goat
- Spec source: internal YAML (kind: synthetic) — hand-authored
- Generation context: regenerate of an existing 2026-04-13 CLI (printing-press 1.3.3) against printing-press 2.3.6 to pick up Surf-Chrome HTTP transport
- Scorecard: 85/100 (Grade A) after polish
- Verify pass rate: 100% (post-polish, was 91% pre-polish)
- Dogfood: PASS post-polish (was WARN with 2 dead helpers + 1 false-positive)
- Phase 5 dogfood matrix: 64/64 PASS, 0 critical
- Fix loops: 1 (shipcheck round 1) + 1 (polish round 1)
- Manual code edits in this session: ~10 files (sites.go, doctor.go, recipe_shared.go, goat_cmd.go, save_cmd.go, root.go, store/store.go, README.md, SKILL.md, brief, manifest)
- Features built from scratch: 0 — all novel features ported from prior 2026-04-13 baseline. Two ranking refinements added (editorial-baseline imputation, Bayesian smoothing) at user request, ~30 lines in goat_cmd.go.

## Findings

### 1. verify-skill specificity-based file picker collides on shared leaf names (Scorer bug)

- **What happened:** The python script `scripts/verify-skill/verify_skill.py:find_command_source` (and its embedded copy `internal/cli/verify_skill_bundled.py`) selects which file declares a cobra command using a "specificity" heuristic — counting required + optional + variadic tokens in the `Use:` string, sorting candidates, and returning files at the highest specificity tier. When two cobra commands share a leaf name at different paths (e.g., `recipe-goat-pp-cli save <url>` and `recipe-goat-pp-cli profile save <name> [--<flag> <value> ...]`), the higher-specificity file wins and the other is dropped. The flag-declaration union check then misses flags declared on the dropped file, producing a false-positive `--tags is declared elsewhere but not on save`.
- **Scorer correct?** **No.** Specificity-based disambiguation is a guess, not a correct algorithm. The right approach is to walk root.go's `AddCommand` graph to determine which file owns each command path, then check that file (plus persistent flags) for declarations.
- **Root cause:** `scripts/verify-skill/verify_skill.py:find_command_source` (and the bundled copy at `internal/cli/verify_skill_bundled.py`) — specificity heuristic at lines ~340-370.
- **Cross-API check:** Recurs on **any CLI with two commands that share a leaf name**. Common patterns: `auth set-token` + a top-level `set-token`; `profile save` + a resource `save`; `cookbook search` + top-level `search`. The recipe-goat case is `profile save` + `save`; this would have hit the prior 2026-04-13 build too if verify-skill had existed.
- **Frequency:** Most CLIs with profile/auth subcommand groups (which is the generator default emitted by every printed CLI today). High recurrence rate.
- **Fallback if the Printing Press doesn't fix it:** Claude can be instructed to watch for this and adjust the Use string to disambiguate, but the workaround is non-obvious and brittle (the bracket-token shape that ties specificity is incompatible with verify's positional-arg inferrer — see Finding 4). Reliability of the human/agent fallback: **low**.
- **Worth a Printing Press fix?** Yes. Both the shared script and the bundled python copy need updating.
- **Inherent or fixable:** Fixable. Walk root.go's AddCommand calls to build the canonical command-path → file map.
- **Durable fix:** Replace `find_command_source` with logic that:
  1. Parses root.go for `rootCmd.AddCommand(newXxxCmd(...))` calls and for each subcommand grouping (e.g. `profileCmd.AddCommand(...)`)
  2. Builds a `command_path → declaring_file` map by following the constructor function names back to their source files (via grep for `func newXxxCmd`)
  3. Returns ONLY the file declaring the requested cmd_path (plus any file containing persistent flag declarations), not all files matching the leaf
- **Test:** Positive: a CLI with `save <url>` at root and `profile save <name>` as subcommand should not raise a false-positive `--<flag> is declared elsewhere but not on save`. Negative: a real flag-on-wrong-command bug must still be caught (e.g., SKILL example uses `--max-time` on `search` when `--max-time` is on `tonight`).
- **Evidence:** Session logs at `internal/cli/save_cmd.go:21` shows the workaround `Use: "save <url> [--tags=<csv>] [--stdin]"` (later changed to `Use: "save [url]"` by polish — see Finding 4 for the conflicting interaction).

### 2. verify positional-arg inferrer treats bracketed-flag tokens as positionals (Scorer bug)

- **What happened:** `internal/pipeline/runtime.go:inferPositionalArgs` extracts positional-arg placeholders from a command's Usage line by matching `[<\[]([a-zA-Z][\w-]*)[>\]]`. When a `Use:` string contains a bracketed flag descriptor like `[--tags=<csv>]`, the regex matches `<csv>` as a positional arg. The verify tool then synthesizes an extra positional value, which can break `cobra.MaximumNArgs(1)` validators on commands that accept exactly one URL.
- **Scorer correct?** **No.** Bracketed tokens in `Use:` strings can be flag descriptors (`[--name=<placeholder>]`) OR optional positionals (`[id]`). The regex doesn't distinguish.
- **Root cause:** `internal/pipeline/runtime.go:582` — `placeholderRe := regexp.MustCompile([<\[]([a-zA-Z][\w-]*)[>\]])`.
- **Cross-API check:** Recurs on any CLI whose `Use:` string includes a bracketed flag descriptor with a placeholder argument. Common shape: `save <url> [--tags=<csv>]`, `query <q> [--limit=<n>]`. Also note this conflicts directly with Finding 1's workaround — the verify-skill workaround style breaks verify.
- **Frequency:** Moderate. Most generator-emitted Use strings are simple (`<positional> [flags]`), but novel-command Use strings often want to advertise key flags inline.
- **Fallback if the Printing Press doesn't fix it:** Use strings can be kept short (just positionals + `[flags]`), but this loses the inline flag hint that helps users. Polishing twice (which we did) is effort.
- **Worth a Printing Press fix?** Yes. Same area as Finding 1; both need a proper Use-string parser.
- **Inherent or fixable:** Fixable. Skip bracketed tokens that contain `--` or `=` or `<flag>=...`.
- **Durable fix:** In `inferPositionalArgs`, before extracting placeholders, strip bracketed tokens that look like flag descriptors. A bracket-token containing `--` (any leading dashes), `=`, or a token starting with `-` should be treated as a flag descriptor and skipped before the placeholder regex runs.
- **Test:** Positive: `Use: "save <url> [--tags=<csv>] [--stdin]"` should infer exactly one positional (`url`). Negative: `Use: "rm <id> [extra]"` should still infer two positionals (`id` required, `extra` optional).
- **Evidence:** Polish-worker output: "Cleaned save command Use string from `save <url> [--tags=<csv>] [--stdin]` to `save [url]` so verify positional-arg inferrer no longer emits a spurious second arg that violated MaximumNArgs(1)".

### 3. dogfood reimplementation_check has no carve-out for novel static-data commands (Scorer bug, partial)

- **What happened:** `dogfood`'s `reimplementation_check` flagged `sub` (the substitution-lookup novel command) as `hand-rolled response: no API client call, no store access`. `sub` reads from a static curated data table in the recipes package — it intentionally has no API or store dependency because the data IS the feature.
- **Scorer correct?** **Partially.** The scorer correctly identified that `sub` has neither client nor store calls. AGENTS.md does say "Hand-rolled response builders that return constants, hardcoded JSON, or struct literals shaped like an API payload" are rejected. But there's a third legitimate carve-out missing: **novel features whose data is the curated content itself** (substitution tables, exclusion lists, seasonal-ingredient maps, etc.).
- **Root cause:** `internal/pipeline/reimplementation_check.go:classifyReimplementation` — only two carve-outs (store signal + client signal); no third for explicitly-marked novel-static commands.
- **Cross-API check:** Recurs whenever a novel feature ships curated reference data. Examples: `holiday list` (US holidays table), `currency list` (curated currency metadata), `iata-codes lookup` (airport database), `unit-convert` (conversion factor tables). All would be flagged today.
- **Frequency:** Subclass — **CLIs with novel reference-data commands.** Roughly 1 in 10 printed CLIs based on a quick scan of the library.
- **Fallback if the Printing Press doesn't fix it:** The scorer issues a WARN, not a FAIL. Ships fine but adds noise. Reliability of fallback: high (ship anyway), but signal-to-noise on dogfood degrades.
- **Worth a Printing Press fix?** Yes — small change, broad applicability.
- **Inherent or fixable:** Fixable.
- **Durable fix:** Two options:
  1. **Manifest-driven:** When `research.json`'s `novel_features[].command` matches the flagged command and the manifest entry has a `category: "static-reference"` or similar tag, exempt automatically. Requires Phase 1.5 to mark these features.
  2. **Code-level:** Recognize a per-command annotation comment like `// pp:novel-static-reference` above the command constructor. Simpler, doesn't require manifest changes.

  Prefer option 2 — local annotation, no upstream dependency on manifest shape.
- **Test:** Positive: a command file with `// pp:novel-static-reference` above `func newSubCmd` is exempted from reimplementation_check even with no client/store calls. Negative: a command without the annotation that has no client/store calls is still flagged.
- **Evidence:** Phase 4 dogfood output: `1/10 novel features look reimplemented: sub (sub_cmd.go) — hand-rolled response: no API client call, no store access`.

### 4. cliutil should expose a site-reachability probe helper (Generator/cliutil enhancement)

- **What happened:** Recipe-goat's hand-extended `doctor.go` originally used HTTP HEAD to probe per-site reachability. Six recipe sites (BBC Good Food, BBC Food, The Kitchn, RecipeTin Eats, AllRecipes, Serious Eats) reject HEAD with TLS shutdown / EOF (Cloudflare/CDN behavior) but serve GET 200 cleanly. Doctor reported them as "unreachable EOF" while the actual scrape commands worked silently — probe drift between doctor and the real fetch path. Fixed mid-session by switching to `GET` with `Range: bytes=0-1023` and treating 416 as reachable.
- **Scorer correct?** N/A — not a scoring finding. The doctor probe is hand-written novel code in recipe-goat (per-site reachability is not a generator-emitted feature).
- **Root cause:** No `cliutil` helper for "is this URL reachable from the CLI's effective HTTP client?" so authors hand-roll HEAD probes that lie.
- **Cross-API check:** Recurs in any multi-source CLI that does per-source reachability checks. The library already has movie-goat, contact-goat, recipe-goat, weather-goat, flightgoat — all multi-source. Some likely have similar HEAD-vs-GET drift if their doctor commands probe sources individually.
- **Frequency:** Subclass — **multi-source CLIs with per-source health checks.** Roughly 5+ in the current library.
- **Fallback if the Printing Press doesn't fix it:** Each multi-source CLI author has to learn the HEAD-EOF lesson independently.
- **Worth a Printing Press fix?** Yes. Add to `cliutil`.
- **Inherent or fixable:** Fixable.
- **Durable fix:** Add `cliutil.ProbeReachable(ctx, client, url) (status string, code int)` to the generator-emitted cliutil package. Implementation: GET with `Range: bytes=0-1023`, discard body, treat 200/206/416 as reachable, 4xx (other) as blocked, network errors as unreachable. Document in cliutil's package comment that doctor commands doing per-source probes should use this helper rather than hand-rolling HEAD.
- **Test:** Positive: a probe of `https://www.bbcgoodfood.com/` reports reachable (real GET 200, HEAD returns EOF). Negative: a probe of `https://example.invalid/` reports unreachable.
- **Evidence:** Session-mid validation: `curl -X HEAD https://www.bbcgoodfood.com/` returns EOF; `curl -X GET https://www.bbcgoodfood.com/ -o /dev/null` returns 200. Six sites in recipe-goat's registry exhibited this pattern.

### 5. Skill should prompt to re-validate prior research when machine version differs (Skill instruction gap)

- **What happened:** This run reused the 2026-04-13 brief (printing-press 1.3.3) without flagging that the machine had since gained Surf-Chrome HTTP transport. The brief's "Tier 1 / Tier 2 / Tier 3 reachability" framing was structurally obsolete (Surf reaches all sites, the tier model was a pre-Surf reachability hierarchy). The user had to explicitly prompt "validate Tier 2/3 status with surf" to catch this. Only after that prompt did we discover Surf reaches everything and re-add 5 sites the prior baseline had excluded.
- **Scorer correct?** N/A.
- **Root cause:** `skills/printing-press/SKILL.md` Phase 0 step 2 ("Check for prior research") and step 4 ("Library Check") tell Claude to "reuse good prior work instead of redoing it" but don't prompt to identify which prior assumptions may have been invalidated by machine changes between the prior run and the current one.
- **Cross-API check:** Every regenerate of an existing CLI where the machine version has changed since the prior run. Common over time as the Printing Press evolves.
- **Frequency:** Every regenerate after a non-trivial machine upgrade.
- **Fallback if the Printing Press doesn't fix it:** User has to know which assumptions are stale and prompt for re-validation. Most users won't know.
- **Worth a Printing Press fix?** Yes — high signal, low cost change to the skill.
- **Inherent or fixable:** Fixable.
- **Durable fix:** In Phase 0 step 4 (Library Check), when the existing `.printing-press.json` manifest's `printing_press_version` is older than the current binary version:
  1. Diff `printing_press_version` (from manifest) vs current binary version (from `printing-press version --json`).
  2. If a non-trivial version delta exists (e.g., minor or major bump), present an explicit prompt before reusing the brief: "The prior run was on 1.3.3. Since then the machine added: [list the major capability deltas pulled from a CHANGELOG or hardcoded list — e.g., 'Surf-Chrome HTTP', 'MCP intent tools']. Want me to re-validate the prior brief's assumptions about [reachability / auth / transport / scoring] against the current machine?"
  3. If the user approves, fold the validation into Phase 1.6 / 1.7 / 1.9 as appropriate.
- **Test:** Positive: rerunning a CLI generated against printing-press 1.x against printing-press 2.x prompts the user about Surf availability before reusing the prior brief. Negative: rerunning on the same minor version doesn't prompt.
- **Evidence:** Session: tier validation only happened after explicit user prompt mid-Phase 4. Brief at `manuscripts/recipe-goat/20260413-091951/research/...-brief.md` predates Surf integration.

### 6. Generator-emitted narrative copy hardcodes runtime-state counts (Generator gap)

- **What happened:** root.go's emitted Short string contains "Find the best version of any recipe across **15 trusted sites**" — pulled from the brief's narrative. But "15 trusted sites" tracks the runtime length of `recipes.Sites`, which expanded from 28→37 in this session. The hardcoded count drifted three-fold and the SKILL/README were also stale. Required hand-edit in 4 places.
- **Scorer correct?** N/A.
- **Root cause:** Generator's narrative pipeline takes the `headline`/`value_prop` strings from `research.json` verbatim and bakes them into root.go's Short/Long, README, and SKILL.md. No mechanism to flag numeric counts that should track runtime state.
- **Cross-API check:** Recurs in any printed CLI whose narrative includes a numeric count tied to a Go slice or table that may grow (multi-source CLIs especially: "across 15 sites", "from 8 retailers", "across 10 leagues").
- **Frequency:** Subclass — **multi-source CLIs**. ~5 in current library.
- **Fallback if the Printing Press doesn't fix it:** Author writes a generic phrasing ("across many trusted sites") in the brief. But the count is genuinely useful info; suppressing it is a worse user experience.
- **Worth a Printing Press fix?** Yes — small, targeted.
- **Inherent or fixable:** Fixable.
- **Durable fix:** Two complementary changes:
  1. **Skill instruction**: Phase 1 brief authoring guidance should say "Avoid hardcoded counts in `headline` / `value_prop` if the count tracks a runtime list. Use plural without count ('across many trusted sites') or use a placeholder the generator will substitute."
  2. **Generator**: support a `{{ site_count }}` style placeholder in `research.json` narrative fields that the generator renders from a configured runtime expression (e.g., `expr: len(recipes.Sites)` or just an integer set in the spec). Niche feature; skill instruction alone may be enough.
- **Test:** Positive: a brief authored with "across {{ site_count }} sites" renders correctly when the spec declares the runtime count. Negative: hardcoded counts in narrative still ship as-is (with a warning during dogfood), so this doesn't break existing CLIs.
- **Evidence:** Session: had to fix "15 sites" → "37 sites" in 4 places (root.go Long, root.go Short, which.go, README, SKILL).

### Skip

- **Ranking-formula refinements (Bayesian smoothing, editorial-baseline imputation).** Recipe-goat-specific. The generator can't prescribe ranking formulas across APIs. Belongs in the printed CLI; not a Printing Press concern.
- **Combo CLI priority audit.** Single-source CLI. No `source-priority.json`. N/A.
- **Phase 4.85 didn't run cleanly.** The agentic output review was preempted by user redirect to validate Tier 2/3 mid-stream. The findings 4.85 would have caught (output plausibility) were caught by the user-led validation instead. Not a Printing Press finding — a normal interactive-flow consequence.

## Prioritized Improvements

### P1 — High priority

| Finding | Title | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---------|-------|-----------|-----------|---------------------|------------|--------|
| F1 | verify-skill specificity-based file picker collides on shared leaf names | `scripts/verify-skill/verify_skill.py` + `internal/cli/verify_skill_bundled.py` | Most CLIs (any with profile or auth subcommand groups) | Low — workaround is non-obvious and conflicts with F2 | Medium (rewrite of `find_command_source` to walk root.go AddCommand graph) | none — apply universally |
| F2 | verify positional-arg inferrer treats bracketed-flag tokens as positionals | `internal/pipeline/runtime.go:inferPositionalArgs` | Subclass: any CLI with bracketed flag descriptors in `Use:` strings | Medium — author can avoid the pattern but loses inline flag hints | Small (add a flag-token filter before placeholder extraction) | none — apply universally |

### P2 — Medium priority

| Finding | Title | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---------|-------|-----------|-----------|---------------------|------------|--------|
| F5 | Skill should prompt to re-validate prior research when machine version differs | `skills/printing-press/SKILL.md` Phase 0 step 4 | Every regenerate after machine upgrade | Low — most users won't know which prior assumptions are stale | Small (add a version-diff prompt) | only fire when manifest `printing_press_version` differs from current |
| F4 | cliutil should expose a site-reachability probe helper | `internal/generator/templates/cliutil_*.go.tmpl` (new file) | Subclass: multi-source CLIs (~5 in library) | Medium — each author has to learn HEAD-vs-GET independently | Small | only meaningful when CLI uses cliutil; opt-in via import |

### P3 — Low priority

| Finding | Title | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---------|-------|-----------|-----------|---------------------|------------|--------|
| F3 | dogfood reimplementation_check has no carve-out for novel static-data commands | `internal/pipeline/reimplementation_check.go:classifyReimplementation` | Subclass: CLIs with curated-data novel commands (~10% of library) | High (WARN, not FAIL — ships anyway) | Small (per-command annotation comment) | annotation must be opt-in to prevent abuse |
| F6 | Generator-emitted narrative copy hardcodes runtime-state counts | Skill (Phase 1 brief authoring) + optional generator placeholder support | Subclass: multi-source CLIs | Medium — generic phrasing works as a fallback | Small (skill instruction); Medium (placeholder support) | skip placeholder support if skill instruction proves sufficient |

## Work Units

### WU-1: Replace verify-skill specificity heuristic with AddCommand graph walk (from F1)

- **Goal:** Eliminate verify-skill false positives on shared leaf names by determining the correct declaring file from `root.go`'s `AddCommand` calls instead of guessing via `Use:`-string token specificity.
- **Target:** `scripts/verify-skill/verify_skill.py` (the canonical script) and `internal/cli/verify_skill_bundled.py` (the bundled copy). Specifically `find_command_source` and any helpers that depend on specificity.
- **Acceptance criteria:**
  - Positive test: a synthetic CLI with `save <url>` at root and `profile save <name>` as a subcommand has SKILL examples on both; verify-skill reports no false-positive flag-commands findings on either path.
  - Positive test: the existing recipe-goat repro at `~/printing-press/library/recipe-goat/internal/cli/save_cmd.go` (with `Use: "save [url]"`) continues to pass without reverting to the bracket-token workaround.
  - Negative test: a real flag-on-wrong-command bug (e.g., SKILL example `search --max-time` when `--max-time` is on `tonight`) is still detected.
  - The bundled copy at `internal/cli/verify_skill_bundled.py` stays in sync with the source script.
- **Scope boundary:** Do not change the rest of verify-skill's check logic (flag-names, positional-args). Do not require manifest input.
- **Dependencies:** None.
- **Complexity:** Medium. Need to parse Go source for `rootCmd.AddCommand(newXxxCmd(...))` and `subCmd.AddCommand(...)` calls and trace constructor function names back to source files.

### WU-2: Filter flag-descriptor tokens out of verify's positional-arg inferrer (from F2)

- **Goal:** Stop verify from extracting `<placeholder>` from bracketed flag descriptors like `[--tags=<csv>]` as if they were positional args.
- **Target:** `internal/pipeline/runtime.go:inferPositionalArgs` (around line 555-595).
- **Acceptance criteria:**
  - Positive test: `Use: "save <url> [--tags=<csv>] [--stdin]"` infers exactly 1 positional (`url`), not 2.
  - Positive test: `Use: "save <url>"` still infers 1 positional.
  - Negative test: `Use: "rm <id> [extra]"` still infers 2 positionals (one required, one optional).
  - Negative test: `Use: "tonight [--max-time <duration>]"` infers 0 positionals.
- **Scope boundary:** Do not change the synthetic-arg-value mapping (`syntheticArgValue`); only change how placeholders are extracted from the Usage line.
- **Dependencies:** None. Independent of WU-1.
- **Complexity:** Small. Add a pre-filter on bracket tokens that contain `--`, `=`, or any leading `-`.

### WU-3: Add `cliutil.ProbeReachable` for multi-source health checks (from F4)

- **Goal:** Stop multi-source-CLI authors from independently rediscovering that some sites reject HEAD but serve GET. Provide a generator-emitted helper that uses GET-with-Range and treats 200/206/416 as reachable.
- **Target:** New generator template `internal/generator/templates/cliutil_probe.go.tmpl` and corresponding output file `internal/cliutil/probe.go` in every printed CLI.
- **Acceptance criteria:**
  - Positive test: `cliutil.ProbeReachable(ctx, client, "https://www.bbcgoodfood.com/")` returns reachable when called from a CLI using the Surf-Chrome client (live; manual or in CI with caveats).
  - Positive test: handles common CDN behaviors: 200 OK, 206 Partial Content (range honored), 416 Range Not Satisfiable (range not honored but headers came back) — all classified reachable.
  - Negative test: `https://example.invalid/` returns unreachable with the original error preserved.
  - The function signature is `ProbeReachable(ctx context.Context, client *http.Client, url string) (status string, code int, err error)` so a doctor command can render `OK`, `WARN`, or `FAIL` per source.
- **Scope boundary:** Do not change existing `cliutil.FanoutRun` or `cliutil.CleanText`. Do not auto-wire ProbeReachable into doctor — authors opt in.
- **Dependencies:** None.
- **Complexity:** Small.

### WU-4: Add manifest-driven re-validation prompt to Phase 0 Library Check (from F5)

- **Goal:** When regenerating a CLI whose prior `.printing-press.json` was stamped by an older binary version, prompt the user about which prior assumptions may need re-validation against the current machine.
- **Target:** `skills/printing-press/SKILL.md` Phase 0 step 4 ("Library Check"). Possibly a small `printing-press machine-deltas <from-version> <to-version>` command if the deltas need to be machine-readable.
- **Acceptance criteria:**
  - Positive test: regenerating a CLI whose manifest stamps printing-press 1.3.3 against current 2.3.6 produces a prompt listing major machine deltas (e.g., "Surf-Chrome HTTP transport added in 2.0", "MCP intent tools added in 2.1") and asks whether to re-validate the prior brief.
  - Positive test: regenerating against the same patch version (e.g., 2.3.6 → 2.3.6) does not prompt.
  - Negative test: a fresh generation (no prior `.printing-press.json`) does not prompt.
- **Scope boundary:** Do not block the user. The prompt is informational + opt-in; declining proceeds with the prior brief unmodified.
- **Dependencies:** None.
- **Complexity:** Small (skill-only) or Medium (if the deltas list needs to be a versioned data file maintained alongside release-please).

### WU-5: Add carve-out to dogfood reimplementation_check for novel static-data commands (from F3)

- **Goal:** Stop dogfood from flagging legitimately-curated static-data novel commands (substitution tables, holiday lists, currency metadata) as reimplementations.
- **Target:** `internal/pipeline/reimplementation_check.go:classifyReimplementation`.
- **Acceptance criteria:**
  - Positive test: a command file with `// pp:novel-static-reference` directive (or similar) above the command constructor is exempted from reimplementation_check even with no client/store calls.
  - Negative test: a command without the annotation that has no client/store calls is still flagged.
  - The annotation is documented in `AGENTS.md` next to the existing Anti-Reimplementation block.
- **Scope boundary:** Do not change other dogfood checks. Do not silently exempt without a directive.
- **Dependencies:** None.
- **Complexity:** Small.

### WU-6: Discourage hardcoded runtime-state counts in narrative copy (from F6)

- **Goal:** Stop generator-emitted narrative copy from drifting when a printed CLI's underlying registry size changes.
- **Target:** `skills/printing-press/SKILL.md` Phase 1 brief-authoring guidance for the `narrative` block in `research.json`. Optionally also `internal/generator/templates/root.go.tmpl` to support a `{{ site_count }}`-style placeholder.
- **Acceptance criteria:**
  - Positive test: skill guidance documents the anti-pattern. New brief authoring sessions avoid hardcoded counts.
  - Positive test: existing CLIs with hardcoded counts continue to ship unchanged (no breaking change).
  - Negative test: dogfood does NOT add a new check for this — it's a soft authoring guideline, not a hard rule. (Avoid scope creep.)
- **Scope boundary:** Skill instruction first. Generator placeholder support is optional and can land in a follow-up WU if the skill instruction proves insufficient.
- **Dependencies:** None. Independent of all other WUs.
- **Complexity:** Small.

## Anti-patterns

- **Specificity-based heuristics for command disambiguation are guesses.** When the language has authoritative source (cobra `AddCommand`), prefer that over Use-string token counting.
- **HEAD probes are unreliable across CDN-fronted sites.** Don't reach for HEAD as the default reachability check; use GET with Range and treat 200/206/416 alike.
- **Two scorers with conflicting demands on the same surface.** verify-skill wanted `Use: "save <url> [--tags=<csv>] [--stdin]"` (specificity tie); verify wanted `Use: "save [url]"` (no fake positionals). Same string, opposite directions. Both scorers had bugs; the workaround chain only ended after both were called out.
- **Reusing prior research without checking machine version.** "Reuse good work" is a sound default; "reuse good work without re-validating against newer machine capabilities" silently ships obsolete assumptions.
- **Numeric counts in narrative that mirror runtime state.** "15 sites" was true once. Now it's wrong in 4 files. Either generate the count at runtime or write the narrative without it.

## What the Printing Press Got Right

- **Surf integration in `client.go.tmpl` was clean and easy to opt into.** Setting `http_transport: browser-chrome` in the spec produced a Surf-Chrome client end-to-end with no further intervention. The dependency landed cleanly in `go.mod` and the existing `*http.Client` consumers (`internal/recipes/fetch.go`) accepted the Surf-equipped client unchanged via the standard `*http.Client` interface — no API surface changes required.
- **`kind: synthetic` worked as designed.** Recipe-goat is a multi-source CLI with USDA as the only API; declaring `kind: synthetic` correctly told dogfood to skip path-validity and the scorecard to omit `path_validity` from the denominator. The generated code still benefited from full novel-feature scoring (10/10 PASS).
- **Polish-worker delivered.** The Phase 5.5 polish agent took a 91% verify / WARN-dogfood input and produced a 100%/PASS output with five targeted code edits, all correctly attributed to the right files. The structured `---POLISH-RESULT---` contract was easy to parse and act on.
- **`lock promote` is atomic and stamps a clean manifest.** No stale state, no missed manifest fields. The 2026-04-13 → 2026-04-26 swap was one command.
- **Per-spec opt-in to browser HTTP transport was the right design.** A USDA-only CLI doesn't need Surf's overhead. Recipe-goat does. The spec's `http_transport: browser-chrome` toggle is the cleanest possible way to express this — no flag explosion, no auto-detection guesswork, just a single declarative field.
