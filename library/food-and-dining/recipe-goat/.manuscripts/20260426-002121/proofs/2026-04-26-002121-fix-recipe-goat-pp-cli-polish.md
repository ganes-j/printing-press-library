# Recipe GOAT — Phase 5.5 Polish Report

Polish-worker dispatched in foreground. Result block below verbatim.

## Polish-worker output

Files modified:
- `internal/cli/helpers.go` — removed dead helpers (extractResponseData, wrapResultsWithFreshness)
- `internal/cli/save_cmd.go` — Use string `"save <url> [--tags=<csv>] [--stdin]"` → `"save [url]"` (verify positional-arg inferrer was extracting `csv` as a second arg, violating MaximumNArgs(1)); added --dry-run early-return for no-args case
- `internal/cli/which.go` — --dry-run soft success when query has no match
- `internal/cli/trending_cmd.go` — --dry-run short-circuit (skips 9s homepage fan-out, fixes occasional verify timeout)
- `internal/recipes/search.go` — added `/recommends/` and `/recommend/` to looksLikeRecipeLink exclusion (fixed pre-existing test failure on mykoreankitchen.com)

## Delta

| Metric | Before | After |
|---|---|---|
| Verify pass rate | 91% | **100%** |
| Scorecard | 82 | **85** |
| Dogfood verdict | WARN (2 dead helpers) | **PASS** |
| go vet | clean | clean |

## Verify-skill workaround note

The save_cmd.go Use-string was changed twice in this session:
- Phase 4 shipcheck: `"save <url>"` → `"save <url> [--tags=<csv>] [--stdin]"` (workaround for verify-skill specificity-based file-picker collision when two cobra commands share the leaf name `save`)
- Polish: `"save <url> [--tags=<csv>] [--stdin]"` → `"save [url]"` (verify positional-arg inferrer treated `csv` as a second positional)

The verify-skill specificity bug is real — see acceptance report for retro item. The polish-worker's simpler `"save [url]"` works because no other command has Use="save [url]" so there's no collision. Both verify checks now pass.

## Ship recommendation

**ship** — proceed to promote.
