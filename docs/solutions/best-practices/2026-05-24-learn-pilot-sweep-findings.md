---
module: sweep-learn-install
tags: [pilot, learn-loop, sweep-tool, findings]
problem_type: validation
---

# U14 pilot sweep findings — sweep-learn-install vs real published-library CLIs

## Summary

U14 of the [generator-wide self-learning CLI plan](https://github.com/mvanhorn/cli-printing-press/blob/main/docs/plans/2026-05-23-002-feat-generator-wide-self-learning-cli-plan.md) is the pilot run of `tools/sweep-learn-install/` against 5 high-value library CLIs. The pilot ran the tool against the requested CLI set and found three classes of blocking defect that prevent the swept output from compiling. No CLI made it past the `go build ./...` gate.

This document captures the per-CLI outcomes and the bugs that surfaced, so the fixes can land in PR #826 (the sweep tool itself) before U14 retries.

## Pilot CLI set

| Position | Original plan CLI | Actual CLI swept | Substitution reason |
|---|---|---|---|
| 1 | `library/media-and-entertainment/espn/` | espn | none |
| 2 | `library/sales-and-crm/contact-goat/` | contact-goat | none |
| 3 | `library/developer-tools/bugbounty-goat/` | `library/developer-tools/company-goat/` | bugbounty-goat is not published in this repo; company-goat is the highest-traffic dev-tools CLI |
| 4 | `library/commerce/instacart/` | instacart | none |
| 5 | `library/media-and-entertainment/podcast-goat/` | podcast-goat | none |

## Per-CLI results

| CLI | Dry-run | Real sweep | Build | Tests | Notes |
|---|---|---|---|---|---|
| espn | OK | wrote 30 learn files + root + store + SKILL + manifest | FAIL | not reached | Bug A + Bug C |
| contact-goat | OK | wrote 30 learn files + root + store + SKILL + manifest | FAIL | not reached | Bug A |
| company-goat | OK | wrote 30 learn files + root + store + SKILL + manifest | FAIL | not reached | Bug B + Bug C |
| instacart | FAIL | sweep refused with `root.go shape unrecognized (no rootFlags type, no var rootCmd)` | n/a | n/a | Expected per plan; instacart has a third root-shape pattern (`func Root() *cobra.Command` with no rootFlags struct) that the detector does not yet recognize |
| podcast-goat | OK | wrote 30 learn files + root + store + SKILL + manifest | FAIL | not reached | Bug B |

All non-instacart writes were reverted with `git checkout -- library/ && git clean -fd library/` before committing this PR. No swept CLI ships in this PR.

## Bugs found in the sweep tool

### Bug A — `store.go` bootstrap inserts `const StoreSchemaVersion = N` **before** the `package` declaration

Affects: any CLI whose `internal/store/store.go` did not already carry a `const StoreSchemaVersion = N` declaration. In this pilot: espn and contact-goat.

In `tools/sweep-learn-install/store_migration.go`'s `ensureStoreSchemaVersion`, the splice locates the package line via `strings.Index(src, "\npackage ")` and then computes `lineEnd := strings.Index(src[pkgIdx:], "\n")` — but `src[pkgIdx]` is itself `\n`, so the second `Index` returns `0` and the insertion happens **at** the position of the `\n`, which puts the const block **before** `package store` and emits source like:

```
// Package store provides ...

// StoreSchemaVersion is the on-disk schema version this binary understands.
const StoreSchemaVersion = 3
package store

import (...)
```

Compile fails with `expected 'package', found 'const'`. The fix is to advance past the package-line `\n` (start the next-`\n` search from `pkgIdx + 1`) and insert after the package declaration rather than before it. The bootstrap also needs to skip past any subsequent blank line so the const lands in conventional position below imports if possible, but at minimum it must land after the package keyword.

### Bug B — root.go AST patch passes `&flags` to `newTeachCmd` et al. when `flags` is already a `*rootFlags` pointer parameter

Affects: any CLI whose `Execute()` / `newRootCmd()` accepts `flags *rootFlags` as a parameter rather than declaring `var flags rootFlags` locally. In this pilot: company-goat and podcast-goat.

The sweep emits:

```go
learnCfg := newLearnConfig()
rootCmd.AddCommand(newTeachCmd(&flags, learnCfg))
rootCmd.AddCommand(newRecallCmd(&flags, learnCfg))
rootCmd.AddCommand(newLearningsCmd(&flags, learnCfg))
rootCmd.AddCommand(newTeachPatternCmd(&flags))
rootCmd.AddCommand(newTeachLookupCmd(&flags))
```

In CLIs that follow the `func newRootCmd(flags *rootFlags)` shape, `flags` is already `*rootFlags`, so `&flags` evaluates to `**rootFlags` and the constructors (which take `*rootFlags`) reject the call:

```
internal/cli/root.go:250:33: cannot use &flags (value of type **rootFlags) as *rootFlags value in argument to newTeachCmd
```

Fix: detect whether the surrounding scope's `flags` identifier resolves to `rootFlags` (value) or `*rootFlags` (pointer) and emit `&flags` or `flags` accordingly. The AST walker already has the function signature in scope when it inserts the `AddCommand` calls, so this is a local decision.

### Bug C — emitted `internal/cli/teach.go` and `internal/cli/learn_init.go` reference helpers that are not present in older library CLIs

Affects: any CLI whose `internal/cli/` does not declare `OpenWithContext` (on store), `dryRunOK`, `printJSONFiltered`, or `parentNoSubcommandRunE`. In this pilot: espn (no `helpers.go` at all) and company-goat (older `helpers.go` shape).

Sample compile errors:

```
internal/cli/learn_init.go:64:19: undefined: store.OpenWithContext
internal/cli/teach.go:181:7: undefined: dryRunOK
internal/cli/teach.go:243:12: undefined: printJSONFiltered
internal/cli/teach.go:421:16: undefined: parentNoSubcommandRunE
```

The sweep's `templates/cli/teach.go.tmpl` and `templates/cli/learn_init.go.tmpl` were lifted byte-for-byte from the cli-printing-press generator emission, which assumes a current `internal/cliutil/` baseline. Older library CLIs (especially anything published before the `cliutil` helpers stabilized) don't carry that baseline, and the sweep tool doesn't emit a shim.

Options for upstream:

1. Emit a sibling `internal/cliutil/learn_helpers.go` (or equivalent) when the host CLI doesn't already declare these helpers. Detect via AST scan of `internal/cli/*.go` and `internal/cliutil/*.go` and only emit when missing.
2. Lower the bar in the templates — replace `OpenWithContext` with `Open` (drop ctx), inline a minimal `dryRunOK` / `printJSONFiltered` / `parentNoSubcommandRunE` literal where used. Keeps the emission self-contained at the cost of duplication when the helpers eventually arrive.
3. Add a pre-flight gate in `sweepCLI` that refuses CLIs lacking the helper baseline (status `skipped: needs cliutil v2 baseline`). Less work for the tool, more work upstream as CLIs trickle into the baseline.

### Auxiliary finding — instacart `Root()` factory shape is a third root-pattern the sweep doesn't recognize

Plan called this out as a watch-item. The current detector understands `var rootCmd` (refuse) and `func Execute() error` with `var flags rootFlags` inside (patch). Instacart uses a third shape: `func Root() *cobra.Command` that builds the command externally with no local `flags` struct.

Adding instacart support means either retrofitting the CLI to one of the two known shapes (sweep-side) or extending the detector to handle the factory shape (tool-side). The plan notes manual retrofit is the expected path for U14; we did not retrofit in this session because Bug A/B/C make the cost-benefit of a manual fix unfavorable until those land.

## Why this PR ships findings, not swept CLIs

The PR-body validation table from the plan ("All 5 pilot CLIs build + test + help + recall smoke pass") cannot be satisfied with the current sweep tool. Three options were on the table:

1. Ship swept CLIs anyway and rely on CI to fail. Rejected — breaks `main` for downstream installs and produces noisy CI for every other PR until reverted.
2. Manually hand-patch each broken CLI to make it compile. Rejected — defeats the purpose of validating the sweep tool against real artifacts. A passing hand-fixed PR would mask the bugs, and the next U15 run would re-hit them across the remaining ~163 CLIs.
3. Document findings, leave swept CLIs out of the diff, retry U14 once PR #826 has fixes for Bug A/B/C. Chosen.

## Phase 2 quantitative thresholds still pending

The plan's Phase 2 stop thresholds (5% false-positive rate, transferability test, dogfood traffic minimum) require 1-2 weeks of dogfood traffic against a working sweep. None of that traffic is generatable today since no CLI was successfully swept. Phase 2 measurement starts after PR #826 lands fixes for the bugs above and U14 retries successfully.

## Recommended next steps

1. Address Bug A, B, C as follow-up commits on PR #826 (or as a sibling PR if the diff gets large).
2. Add unit tests in `tools/sweep-learn-install/` covering:
   - `ensureStoreSchemaVersion` against a file that does not already declare the constant (regression for Bug A).
   - `patchRootAST` against a `func newRootCmd(flags *rootFlags)` host (regression for Bug B).
   - `planSweep` against a CLI whose `internal/cli/` lacks `OpenWithContext` / `dryRunOK` / `printJSONFiltered` / `parentNoSubcommandRunE` (regression for Bug C — should refuse cleanly or emit shims).
3. Once fixed, re-run the U14 pilot in this same branch shape (4 CLIs + instacart manual or refused).

## Files exercised

- Sweep tool: `tools/sweep-learn-install/{main.go, store_migration.go, root_ast.go, learn_files.go, templates/cli/teach.go.tmpl, templates/cli/learn_init.go.tmpl}` from PR #826 commits `612d9f97`, `86d1d346`.
- Pilot CLIs (no diff in this PR): `library/media-and-entertainment/espn/`, `library/sales-and-crm/contact-goat/`, `library/developer-tools/company-goat/`, `library/commerce/instacart/`, `library/media-and-entertainment/podcast-goat/`.

## v2 results (after PR #826 fixes for Bug A/B/C)

Following PR #826 fixes for bugs A, B, C, this section records the v2 pilot outcomes. The pilot branch was rebased onto the fixed `feat/sweep-learn-install` tip (which now carries commits `2080be2b` Bug A fix, `a0b22a78` Bug B fix, `0709363b` Bug C fix), the sweep tool was rebuilt, and the same 5 pilot CLIs were re-run.

**Headline:** Bugs A, B, C are confirmed fixed — every previously-broken `go build ./...` step now passes for the 4 expected-to-succeed CLIs. Instacart still refuses with the new clean factory-shape diagnostic from the Bug B fix, as the plan predicted. **However, the sweep surfaced a new blocker, Bug D, which prevents any swept CLI from running its migrations at runtime.** All 4 swept pilots were reverted; PR #827 continues to ship only the findings doc.

### Per-CLI v2 outcomes

| CLI | Dry-run | Real sweep | `go build ./...` | `go test ./...` | `--help` shows teach/recall/learnings | `recall "smoke test" --agent` | Notes |
|---|---|---|---|---|---|---|---|
| espn | OK | wrote 30 learn files + root + store + SKILL + manifest | PASS | PASS (155/155) | PASS | **FAIL** (Bug D, runtime) | Migration: `SQL logic error: incomplete input (1)` |
| contact-goat | OK | wrote 30 learn files + root + store + SKILL + manifest | PASS | PASS (412/412) | PASS | **FAIL** (Bug D, runtime) | Same migration error |
| company-goat | OK | wrote 30 learn files + root + store + SKILL + manifest | PASS | **FAIL** (4 store-test failures + multiple cli tests) | PASS | **FAIL** (Bug D, runtime) | `TestSchemaVersion_StampedOnFreshDB` and 3 others fail in `internal/store` with same migration error |
| podcast-goat | OK | wrote 30 learn files + root + store + SKILL + manifest | PASS | **FAIL** (10+ store-test failures) | PASS | **FAIL** (Bug D, runtime) | Same migration error surfaces in many store tests because `Open()` migrate failures abort fixture setup |
| instacart | n/a (refused) | refused with `root.go uses the func Root() *cobra.Command factory shape with no rootFlags struct (recognized but unsupported by auto-sweep; manual retrofit required, see tools/sweep-learn-install/README.md)` | n/a | n/a | n/a | n/a | **Expected** — refusal diagnostic is now actionable and points at the manual-retrofit README section per plan |

espn and contact-goat tests pass because their store tests do not exercise the freshly-spliced learn migrations directly — those CLIs' suites don't open `internal/store` with a fresh DB. company-goat and podcast-goat have store-test suites that do, so the bug shows up there.

All 4 swept CLIs were reverted with `git checkout -- library/<cat>/<cli>/ && git clean -fd library/<cat>/<cli>/` before this commit. PR #827 continues to ship the findings doc only.

### Bug D — canonical learn-migrations block emits CREATE TABLE statements missing the outer `)`

Affects: every CLI swept by the post-A/B/C-fix `feat/sweep-learn-install`. The Go source-level splice succeeds, the file compiles, but the SQL emitted at migration time is malformed.

In `tools/sweep-learn-install/store_migration.go`'s `canonicalLearnMigrationsBlock`, each `CREATE TABLE` statement is closed with `\n\t\t\``,\n` (raw-string close + backtick-comma) instead of `\n\t\t)\``,\n`. Concretely, lines 64–65:

```go
PRIMARY KEY (query_pattern, resource_type)
` + "`,\n" +
```

emit:

```
PRIMARY KEY (query_pattern, resource_type)
		`,
```

— but the matching `(` after `CREATE TABLE IF NOT EXISTS search_learnings` is never closed. The same is true for `search_patterns`, `entity_lookups`, `teach_log_metadata`, and `search_learnings_fts`. Cross-checking against the upstream template at `cli-printing-press/internal/generator/templates/store.go.tmpl`, the canonical text on disk in the sweep tool is missing the closing `)` for all 5 statements that the generator emits correctly.

At runtime, `s.migrate()` runs the first malformed statement and returns `SQL logic error: incomplete input (1)`. The CLI cannot open a fresh database, so `recall`, `teach`, `learnings`, `teach-pattern`, `teach-lookup`, and every other command that opens the store, all fail. Existing databases that already carry the legacy schema continue to fire a different SQL error (`no such column: season_year` on espn, for example) because the second migration tries to add the learn tables and bombs partway through.

Fix: update `canonicalLearnMigrationsBlock` in `tools/sweep-learn-install/store_migration.go` to include `\n\t\t)` immediately before each `\``,\n` boundary (5 places), and add a regression test in `store_migration_test.go` that runs the emitted migrations against an in-memory `modernc.org/sqlite` database and asserts a successful `s.migrate()` end-to-end.

### Status going into Phase 2

Phase 2 quantitative thresholds remain not measurable. Bugs A/B/C fixes unblocked compilation, but Bug D blocks every runtime path that opens the store. Once #826 lands a Bug D fix (one-line patch + regression test against the migrations slice) and the pilot retries with green `recall --agent` smokes, dogfood traffic can finally start accumulating and Phase 2 thresholds become measurable.

**Update to "Phase 2 quantitative thresholds still pending":** Thresholds are still not measurable — Bug D fix gates measurement, same as Bug A/B/C did. The "1-2 weeks of dogfood traffic starts now" milestone is deferred until #826 ships the Bug D fix and U14 retries with passing runtime smokes across all 4 expected-to-succeed CLIs.

### Recommended next step for #826

Add a fourth bug-fix commit on `feat/sweep-learn-install`:

```
fix(sweep-learn): close CREATE TABLE statements in canonicalLearnMigrationsBlock
```

with a regression test that asserts `Open()`-then-`migrate()` succeeds against a fresh in-memory SQLite database whose `migrations` slice has been built from the canonical block. The expected diff is +5 lines of `)` and one new test function.

Once that lands, retry U14 in this same branch shape (rebase pilot onto fixed tip, re-run sweep, validate 4 CLIs, leave instacart refused, ship per-CLI commits).
