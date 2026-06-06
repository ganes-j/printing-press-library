---
title: "feat: ppl as the 'brew of CLIs' + multi-channel distribution (Homebrew, NPM, curl | sh)"
type: feat
status: active
date: 2026-04-19
deepened: 2026-04-19
---

# feat: ppl as the "brew of CLIs" + multi-channel distribution

## Overview

Printing Press today produces great CLIs but ships them through a narrow pipe: `go install` (requires a Go toolchain) or `/plugin install` (requires Claude Code). Every `.goreleaser.yaml` is written, but nothing is actually released. Homebrew formulas are referenced but the tap does not exist. NPM is not wired up at all.

This plan turns the Printing Press Library into the "brew of CLIs":

1. Publish real prebuilt binaries for every library CLI, cross-platform, signed, on GitHub Releases.
2. Ship them through three public channels: a Homebrew tap, scoped NPM packages with a platform-dispatch wrapper, and a curl | sh bootstrap.
3. Ship a standalone `ppl` binary that acts as the package manager: `ppl install espn`, `ppl search sports`, `ppl upgrade`, `ppl tap add owner/repo`. The existing `/ppl` Claude Code skill becomes a thin shell over the binary.
4. Treat taps as first-class from day one: the library's `registry.json` is the official tap, and any git repo with a conformant `registry.json` can be added as a third-party tap.

The goal is that a user with no Go toolchain and no Claude Code can type `brew install espn-pp-cli` or `npx @printing-press/espn scores` or `curl -fsSL get.printingpress.dev | sh && ppl install espn` and get the same tool an agent can.

## Problem Frame

Today's install paths:

- Generator: `go install github.com/mvanhorn/cli-printing-press/cmd/printing-press@latest`
- Library CLI: `go install github.com/mvanhorn/printing-press-library/<path>/cmd/<binary>@latest`
- Claude Code: `/plugin install cli-printing-press@cli-printing-press` or the library equivalent

Every CLI in the library carries a working `.goreleaser.yaml` (verified in five `library/*/.goreleaser.yaml` files: espn, dominos-pp-cli, yahoo-finance, kalshi, linear, weather-goat). Each config declares Homebrew publishing targeted at `trevin-chow/homebrew-tap`. There is no release workflow and no tap repo, so nothing ships. The `/ppl` skill hardcodes `go install` because that is the only path that works.

Consequences:

- Non-Go users cannot install a printed CLI without compiling from source.
- Non-Claude Code users cannot discover or install anything short of reading the README.
- No update mechanism beyond re-running `go install @latest`. No uninstall (the skill explicitly tells users to delete binaries manually).
- Third-party contributors have no path to publish a CLI to the ecosystem short of a PR to this repo.

The "brew of CLIs" framing targets three properties Homebrew has that we lack:

| Brew property | Current PP state |
|---------------|------------------|
| One command install with prebuilt binaries | Requires Go toolchain |
| Central curated catalog plus pluggable taps | Registry is central-only; tap concept does not exist |
| Search, list, upgrade, uninstall, info, doctor | Only install, and only via `go install` |
| Platform-agnostic bootstrap (`curl install.sh \| bash`) | None |

## Requirements Trace

- R1. A user without Go or Claude Code can install any library CLI via `brew install`, `npm i -g`, or `npx` on macOS, Linux, and Windows.
- R2. The `ppl` command is a real binary runnable from any shell, offering install, uninstall, search, list, upgrade, info, doctor, and tap management.
- R3. Third-party taps are supported: any git repo with a conformant `registry.json` can be added with `ppl tap add <owner>/<repo>` and its CLIs become installable.
- R4. Every printed CLI in `printing-press-library` has an automated release that publishes GitHub Release artifacts, updates the Homebrew tap, publishes to NPM, and updates `registry.json` fingerprints on tag push.
- R5. The official Homebrew tap is `mvanhorn/homebrew-printing-press`; all per-CLI `.goreleaser.yaml` files are updated to point there.
- R6. NPM packages for our CLIs use the scoped `@printing-press/*` namespace with an esbuild-style platform-dispatch wrapper so `npx @printing-press/espn` works on every supported platform without postinstall network calls.
- R7. A single `curl -fsSL <url> | sh` command installs the `ppl` binary and adds it to `PATH` (via `~/.pp/bin`). Source URL is either a GitHub raw URL (phase 1) or `get.printingpress.dev` (phase 2 if a domain is registered).
- R8. The Claude Code `/ppl` skill continues to work end-to-end but switches to `ppl install` under the hood when the binary is available, and falls back to `go install` when it is not.
- R9. Tag schema, registry schema, and NPM package schema are versioned and documented so future shape changes are non-breaking.
- R10. No automation in this plan talks to printed-CLI auth, SQLite stores, or compound commands. Distribution is orthogonal to the data layer.

## Scope Boundaries

In scope:

- Release pipeline per library CLI (GitHub Actions + goreleaser).
- Homebrew tap repo and formula publication.
- NPM publishing infrastructure and the generator that produces per-CLI package shells from `registry.json`.
- Standalone `ppl` binary (new cmd in `cli-printing-press`).
- curl | sh installer script for `ppl`.
- Tap protocol v1: git repos with `registry.json`.
- Cross-wiring: `/ppl` skill delegates to `ppl` binary when present.

Out of scope (explicit non-goals):

- Changing how any printed CLI resolves auth, reads env vars, or talks to its API. Distribution only.
- A hosted registry service or web dashboard. The catalog stays in git.
- Binary signing with a new Apple Developer ID or Windows Authenticode setup. We rely on goreleaser checksums and SLSA provenance via GitHub's OIDC. Notarization can be a phase-three add.
- Rewriting the generator to produce NPM shells directly. The NPM generator runs over `registry.json` post hoc, not inside the generator.
- Migrating existing users away from `go install`. It stays working; the new paths are additive.
- Telemetry on install events. No phoning home.

## Context & Research

### Relevant Code and Patterns

- `/Users/mvanhorn/cli-printing-press/.goreleaser.yaml`: top-level goreleaser that builds `printing-press` and `printing-press-mcp` for linux/darwin/windows, amd64/arm64. No `brews:` stanza. Model to copy when adding the tap target.
- `/Users/mvanhorn/printing-press-library/library/media-and-entertainment/espn/.goreleaser.yaml`: per-CLI goreleaser with working `brews:` block targeting `trevin-chow/homebrew-tap`. All per-CLI goreleaser files in `library/**/.goreleaser.yaml` follow this shape.
- `/Users/mvanhorn/printing-press-library/registry.json`: the catalog. `schema_version: 1`, one entry per CLI with `name`, `path`, optional `mcp` block. This is the tap schema.
- `/Users/mvanhorn/printing-press-library/plugin/skills/ppl/SKILL.md`: today's `/ppl` router. Modes 1 through 5 (discovery, CLI install, MCP install, explicit use, semantic use). Install paths all use `go install`. Modes 2 and 3 are the ones that need to change when `ppl` binary is present.
- `/Users/mvanhorn/printing-press-library/.github/workflows/generate-skills.yml`: the pattern for registry-driven generation on push. Release workflow should mirror this shape.
- `/Users/mvanhorn/cli-printing-press/cmd/`: precedent for adding a new binary alongside `printing-press` and `printing-press-mcp`.
- `/Users/mvanhorn/cli-printing-press/docs/plans/2026-04-19-001-feat-composio-inspired-features-plan.md`: sibling plan that adds a super-CLI for execute/search/info/proxy. That plan lives in the generator binary under `printing-press run ...`. This plan's `ppl` binary is orthogonal (package manager, not runtime) and should not collide.

### Institutional Learnings

- From memory: always PR before main, never push directly. Every release workflow must PR the tap update, not push directly, for the first release cycle so we can review the diff before it goes live.
- From memory: always detect npm package status before presenting merge options on owned repos. The NPM unit must start with an availability check on the `@printing-press` scope and each candidate package name.
- From memory: `/ppl` skill is delivered via the public `mvanhorn/printing-press-library` repo, not the private variant. Release workflow changes must land in the public repo.
- From memory: no CronCreate-style monitoring for long-running workflows. Release verification is manual and synchronous: tag, wait for the workflow, then verify.
- From `mvanhorn/open-source-contributor` (OSC) NPM tooling audit: four patterns transfer directly and save real work in Unit 5 and Unit 6:
  - `skills/osc-status/handlers/handle-npm-failure.md`: workflow-log failure classification (version already published, OIDC misconfig, etc.) with targeted fixes. The `npm view <pkg> version` "version-exists guard" transfers as-is. Reuse in Unit 5's error paths.
  - `skills/osc-status/handlers/pre-push-verification.md`: language-agnostic pre-push build/lint/test gate with escalating caution on repeated failures. Reuse verbatim for the `ppl` repo and the release workflow.
  - `skills/osc-status/handlers/post-push-monitor.md` (scheduled via `docs/plans/2026-04-07-001-feat-universal-post-push-monitor-plan.md`): shared post-push release verification handler. Reuse for Unit 6 release workflow verification.
  - `docs/plans/2026-03-30-006-fix-npm-detection-structural-gate-plan.md`: the critical learned lesson. See KTD-25.
- From the same OSC audit, what does NOT transfer: `npm version` bump choreography (we use Git tags, not `package.json`), the npm-specific OIDC preflight (our OIDC is NPM + Trusted Publishing on scoped binary packages, different checks), and `fetch_npm_metadata()` in `osc-collect.py` (we read `registry.json` + `releases.json`, not `package.json`). Unit 5 builds those npm-specific pieces fresh rather than porting OSC's paperclip-plugin code.

### External References

- Homebrew tap conventions: `mvanhorn/homebrew-printing-press` is brew-canonical. Users run `brew tap mvanhorn/printing-press` and `brew install espn-pp-cli`. Formulas live under `Formula/*.rb`.
- esbuild / biome / turbo NPM pattern: one scoped meta package with a stub bin that re-execs the platform-specific `@scope/<name>-<os>-<arch>` optional dependency. NPM resolves only the matching platform at install. Zero postinstall network calls. Works with npx. Reference: esbuild's `npm/` directory layout.
- GoReleaser Homebrew publishing: `brews:` stanza pushes a generated formula as a commit or PR to the tap repo via a GitHub token with write access. Multi-binary formulas install each binary into `bin/`.
- curl | sh installer baseline: detect `uname -s` and `uname -m`, map to a goreleaser archive name, download from GitHub Release `latest`, verify against `checksums.txt`, install to `~/.pp/bin/ppl`, print PATH guidance. No sudo.

## Key Technical Decisions

KTD-1. `ppl` is a new standalone binary, not a subcommand of `printing-press`. Rationale: the generator is for CLI authors; the package manager is for CLI users. They have different audiences, different install cadences, and different blast radius when something goes wrong. Mirroring brew's shape (a dedicated client binary) is also what "brew of CLIs" means literally. The `/ppl` Claude Code skill remains the high-level entry point and now has a real binary to shell out to.

KTD-2. Superseded by KTD-16 after architectural review. The `ppl` binary source lives in its own `mvanhorn/ppl` repo, not `cli-printing-press/cmd/ppl`. See KTD-16.

KTD-3. Superseded by KTD-20 after architectural review. The catalog is split into `registry.json` (stable catalog metadata) and `releases.json` (volatile distribution metadata). See KTD-20.

KTD-4. Tap protocol v1 is "git repo plus `registry.json` at repo root". No signing, no webhooks, no central coordination. `ppl tap add <owner>/<repo>` clones to `~/.pp/taps/<owner>-<repo>/` and reads `registry.json`. `ppl tap update` runs `git pull` across all taps. Rationale: git is the universal tap transport and is how Homebrew itself works. It also means taps are auditable (diffable) and forkable. Signing and trust policies are a phase-three add once real third-party taps exist.

KTD-5. Binary dispatch order when resolving `ppl install <name>`: official tap (mvanhorn/printing-press-library) wins on name collisions unless the user passes `--tap <owner>/<repo>`. Rationale: users who type `ppl install espn` expect the curated thing, not whatever third-party tap they added last. Collisions are surfaced in `ppl search <name>` output with tap source per row.

KTD-6. NPM distribution pattern is esbuild-style platform dispatch: `@printing-press/<cli>` is the meta package, with `@printing-press/<cli>-{darwin,linux,win32}-{x64,arm64}` as optional dependencies. The meta package's `bin` stub `require`s the platform optional dep and re-execs its binary path. Rationale: postinstall hooks are opt-out in enterprise environments, break in offline installs, and require a network round-trip at install time. Platform dispatch runs entirely inside NPM's resolver. Reference implementation: esbuild.

KTD-7. NPM scope is `@printing-press/*`. Rationale: clean brand, one org to manage. If the scope is taken, fall back to `@pp-library` or `@ppcli`. Availability check is Unit 5's first task.

KTD-8. Homebrew tap repo is `mvanhorn/homebrew-printing-press`. User-facing name becomes `brew tap mvanhorn/printing-press`. Rationale: the user chose this explicitly. All per-CLI `.goreleaser.yaml` files are updated to point at this repo in the same PR as the tap is created.

KTD-9. Release cadence is per-CLI tags, not monorepo tags. Tag format: `<cli-name>/v<semver>`, e.g. `espn-pp-cli/v1.2.0`. The release workflow is keyed off tag prefix to run only that CLI's goreleaser. Rationale: 21 CLIs evolve independently; a monorepo tag would force lockstep versioning and make per-CLI rollbacks painful. The `ppl` binary itself is tagged as `ppl/vX.Y.Z` on the `cli-printing-press` side.

KTD-10. The curl | sh installer only installs the `ppl` binary. It does not pre-install any library CLI. Rationale: the installer stays small and auditable; `ppl install <name>` is where choice happens. This also matches brew's shape: `install.sh` installs brew, not formulas.

KTD-11. Installer URL in phase 1 is `https://raw.githubusercontent.com/mvanhorn/printing-press-library/main/install.sh`. A vanity domain (`get.printingpress.dev`) is deferred to phase 3 only if someone wants to register it. Rationale: a domain adds DNS + HTTPS operational surface. Raw GitHub is free, cacheable, and verifiable.

KTD-12. `/ppl` skill delegates to the `ppl` binary with a capability probe. `which ppl` in step 0 of each mode; if present, call `ppl install`, `ppl search`, etc.; if absent, fall back to the current `go install` path. Rationale: zero breakage for existing users during rollout. The skill can detect and recommend the binary once it is published.

KTD-13. Binaries in release archives are unsigned in phase 1. macOS users will see Gatekeeper prompts the first run; mitigation is a one-line xattr instruction in `ppl doctor`. Rationale: Apple Developer ID requires a $99/yr account and notarization CI is non-trivial. Gate on real demand before paying that cost. Linux and Windows do not need this.

KTD-14. `ppl upgrade` is per-CLI: it re-resolves the manifest, compares versions, and downloads the new binary. `ppl upgrade --all` iterates. There is no lockfile and no atomic rollback for v1. Rationale: brew itself does not offer atomic rollback; users trust the tap. We should not reinvent package management before the base case works.

KTD-15. Binary install target is `~/.pp/bin/`, not `/usr/local/bin/`. Rationale: no sudo, no conflict with system package managers, easy uninstall (`rm -rf ~/.pp`). `ppl doctor` warns when `~/.pp/bin` is not on `PATH` and prints exact shell rc lines to add. Path-insertion policy: append `~/.pp/bin` to PATH, never prepend, so `ppl`-installed binaries cannot shadow system tools like `git`, `ssh`, `curl`, `node`, `python`, `brew`, or `sudo`.

KTD-16. Supersedes KTD-2. `ppl` lives in a new `mvanhorn/ppl` repo with its own `cmd/ppl`, `internal/`, `.goreleaser.yaml`, and release workflow. Rationale: `ppl` has independent release cadence from the generator (a tap-resolver bug should not force a `printing-press` bump), different audience (end users vs CLI authors), different blast radius (a bad `ppl` release breaks every user's `brew upgrade`; a bad generator release only breaks the next `printing-press generate`), and `cli-printing-press/AGENTS.md` is explicit about machine-vs-printed discipline. A shared manifest reader is vendored via Go module so the tap schema stays consistent across `ppl`, megamcp, and the skill generator. This change also eliminates the KTD-9 tag collision between `ppl/vX.Y.Z` and generator-repo tags.

KTD-17. Third-party tap clones are hardened against malicious git configuration. Rationale: the tap protocol is a full git clone into `~/.pp/taps/`, so a malicious tap can set `core.fsmonitor`, `core.sshCommand`, `core.hooksPath`, or a `[includeIf]`-referenced config that runs arbitrary code on the next `git pull`. Every clone uses `--no-local --config protocol.file.allow=never --config core.hooksPath=/dev/null --config core.fsmonitor=false`. Every `git pull` runs with the same `-c core.hooksPath=/dev/null -c core.fsmonitor=false`. Tap remotes that do not point to `https://github.com/` are rejected in v1. Tap registry files must be `registry.json` at repo root; nothing else in the clone is executed by `ppl`.

KTD-18. Trust root is explicit and anchored outside the tap. Rationale: the plan's original wording ("verify sha256 from registry AND checksums.txt") conflates two dependent sources; a compromised tap owner controls `registry.json`, and a compromised `mvanhorn` GitHub account controls `checksums.txt` in the same release. For the official tap, `ppl` requires (a) `checksums.txt` from GH Releases, (b) sha256 in `releases.json` matches the checksums entry, and (c) SLSA provenance attestation signed via GitHub's keyless OIDC (cosign `verify-blob --certificate-identity`). Mismatch at any step is exit 5 (possible tampering). SLSA provenance + cosign verification moves from "out of scope" to phase 2; it is free via GitHub OIDC and closes the two biggest supply-chain holes in one move. For third-party taps in v1, the tap's `releases.json` sha256 is the trust root and installs emit a "unsigned, tap-trusted only" banner on first use.

KTD-19. Installs from non-official taps require explicit trust. Rationale: official-tap-wins-collisions (KTD-5) only defends against name-squatting on popular CLIs. A user who runs `ppl tap add evil/tap` then `ppl install unique-name` would silently install an unsigned binary from an untrusted source. First install from any non-official tap prints the tap URL, last-updated timestamp, and the release sha256, then requires `[y/N]` confirmation. The acknowledgement is recorded in `~/.pp/pkg/state.json` as `trusted_taps[]`; subsequent installs from that tap skip the prompt. `ppl tap trust <owner>/<repo>` and `ppl tap revoke <owner>/<repo>` manage the list. `ppl install --from <tap>` bypasses the official-first resolver for scripted use.

KTD-20. Supersedes KTD-3. The catalog is split into two files, both read by `ppl`, megamcp, and the skill generator:

- `registry.json`: stable catalog metadata. Fields: `name`, `category`, `api`, `description`, `path` (internal to the official tap only, not part of the third-party tap contract), `mcp` block. Rarely changes; PRs here are product decisions.
- `releases.json`: volatile distribution metadata. Fields: per-entry `latest_release`, `versions[]`, `bottles{os-arch: {asset_url, sha256, size}}`, `npm_package`, `homebrew_formula`, `published_at`, keyed by registry `name`. Mutates on every per-CLI release. Entries are append-only and keyed by name, not array position, so concurrent release workflows do not merge-conflict.

Both files carry `schema_version: 1`. `ppl`'s tap validator refuses unknown major versions. Rationale: three consumers already read `registry.json`; adding distribution fields plus per-CLI release PRs creates merge-conflict pressure against skill generator PRs and manifest regeneration. Splitting also lets the tap protocol drop distribution fields from the catalog contract entirely.

KTD-21. The third-party tap schema drops `path`. Rationale: `path: "library/media-and-entertainment/espn"` is meaningful only because the official tap happens to be a monorepo; a third-party tap wrapping a single external CLI has no `library/**/cmd/<binary>` structure. The tap entry contract is `{name, description, category, source: {type: "github_release", owner, repo}, artifact_name_template}` plus optional `mcp`. The monorepo-specific `path` stays as an internal field in the official tap's `registry.json` for `tools/generate-skills`, but is not part of the tap-v1 contract. `docs/taps.md` ships a minimal third-party tap example with zero `library/**` directories so the schema boundary is enforced by documentation and by Unit 9's validator.

KTD-22. The shared `~/.pp/` namespace is carved between ppl and the sibling Composio-plan runtime. Layout: `~/.pp/bin/` (shared binary output, both plans write here), `~/.pp/pkg/{state.json,taps/,cache/}` (ppl-owned, this plan), `~/.pp/auth/{credentials.json,.device-key,.lock}` (Composio-owned), `~/.pp/runtime/{triggers.json}` (Composio-owned). Both plans check the layout in `doctor`. Rationale: both plans assume ownership of `~/.pp/` today; without a carveout they will collide on `chmod` semantics, `uninstall` scope, and doctor checks within a week of shipping together.

KTD-23. `ppl` package-manager verb is `find` (or `browse`), not `search`. Rationale: the sibling Composio plan uses `printing-press run search` for tool-level discovery (searches 60+ tools across activated APIs). Using `ppl search <api>` for package-level discovery (one row per matching CLI) would return confusingly different results to users and agents who typed it expecting the other semantic. Renaming to `ppl find <query>` reserves `search` for the tool-level semantic. `/ppl` skill help text cross-links: "to find CLIs: `ppl find <query>`; to find tools inside an installed CLI: `printing-press run search <query>`".

KTD-24. NPM publishing uses OIDC Trusted Publishing, not a long-lived `NPM_TOKEN`. Rationale: npm Classic Tokens were permanently deprecated December 9, 2025; Granular tokens max out at 90 days and require 2FA rotation. OIDC Trusted Publishing (GA July 2025) eliminates stored secrets, automatically attaches SLSA provenance to every publish, and keys publish permission to the specific GitHub repo + workflow file. Removes the token-leak risk entirely and aligns with KTD-18's provenance story.

KTD-25. Release-flow decision points are enforced with mandatory structural gates, not prose instructions. Rationale: OSC hit this exact class of bug on 2026-03-30 (`open-source-contributor/docs/plans/2026-03-30-006-fix-npm-detection-structural-gate-plan.md`). Prose instructions like "also check if this is an npm package" compete with other prose and get rationalized-away under pressure, especially in batch workflows. A prefixed banner that must print before the next step proceeds cannot be skipped. The release flow for this plan has three such gates, all implemented as banner-print-then-block patterns in Unit 5 and Unit 6:

- `=== CLI PACKAGE DETECTION GATE ===` -> fires before any merge offer on a library CLI PR; verifies the CLI has `.printing-press.json` and a `.goreleaser.yaml` before the flow can continue.
- `=== RELEASE WORKFLOW PREFLIGHT ===` -> fires before tag push; verifies `.goreleaser.yaml` targets the right tap, NPM generator can emit the package shells, and release workflow has the right permissions.
- `=== RELEASE VERIFICATION GATE ===` -> fires after tag push; blocks the thank-you/close step until GH Release archives, brew formula PR, NPM platform packages, and releases.json PR all exist. This gate wraps the post-push-monitor pattern from OSC (`open-source-contributor/skills/osc-status/handlers/post-push-monitor.md`).

These gates protect the fan-out in Unit 8 specifically, where 21 CLIs release and the pressure to skip verification on each one is highest.

## Open Questions

### Resolved During Planning

- Should `ppl` be a subcommand of `printing-press` or a new binary? New binary. Different audience, different cadence, real brew parity. (KTD-1)
- Where does `ppl` source live? `cli-printing-press/cmd/ppl`. Generator repo already has release infrastructure. (KTD-2)
- Extend `registry.json` or introduce a new schema? Extend. Additive fields only, schema_version bump only if breaking. (KTD-3)
- What is the tap protocol? Git repo plus `registry.json`. No signing in v1. (KTD-4)
- NPM install pattern? esbuild-style platform dispatch via optionalDependencies. (KTD-6)
- Homebrew tap repo name? `mvanhorn/homebrew-printing-press` (user confirmed). (KTD-8)

### Deferred to Implementation

- Exact `@printing-press` scope availability. If taken, fall back per KTD-7. Unit 5 checks first.
- Final NPM package name shape: `@printing-press/espn` vs `@printing-press/espn-pp-cli`. Unit 5 decides based on registry naming.
- Notarization cost/benefit for macOS binaries. Revisit once install numbers justify the $99/yr. (KTD-13)
- Domain registration for `get.printingpress.dev`. Phase 3 only. (KTD-11)
- Exact `ppl doctor` rubric. Known checks: PATH contains `~/.pp/bin`, `git` available for taps, taps are less than 7 days stale, installed binaries match manifest checksums. More discovered during Unit 7 implementation.
- Binary cache strategy for downloads (ETag, If-Modified-Since). v1 just downloads; caching is a follow-up.

## High-Level Technical Design

> This illustrates the intended approach and is directional guidance for review, not implementation specification. The implementing agent should treat it as context, not code to reproduce.

Distribution topology after shipping:

```
                      mvanhorn/cli-printing-press
                      +----------------------------+
                      | cmd/printing-press (gen)   |
                      | cmd/printing-press-mcp     |
                      | cmd/ppl (NEW)              |
                      +-------------+--------------+
                                    |
                                    | tag push -> goreleaser
                                    v
                      +-------------+--------------+
                      | GitHub Releases             |
                      | - ppl_{os}_{arch}.{tar.gz,  |
                      |   zip} + checksums.txt      |
                      +-------------+--------------+
                                    |
              +---------------------+---------------------+
              |                     |                     |
              v                     v                     v
     +-----------------+   +-----------------+   +------------------+
     | Homebrew tap    |   | curl | sh       |   | NPM @printing-  |
     | mvanhorn/       |   | (installs ppl   |   | press/* (ppl    |
     | homebrew-       |   | from GH Release)|   | meta + platform |
     | printing-press  |   +-----------------+   | optional deps)  |
     +--------+--------+                         +--------+---------+
              |                                           |
              | brew install ppl                          | npx @printing-press/ppl
              v                                           v
           user's laptop <--- ppl install espn --- user's laptop


                   mvanhorn/printing-press-library
                   +-------------------------------+
                   | registry.json (the tap)       |
                   | library/**/.goreleaser.yaml   |
                   | library/**/cmd/<binary>/      |
                   +---------------+---------------+
                                   |
                                   | tag <cli>/vX.Y.Z push
                                   v
                   +---------------+---------------+
                   | per-CLI release workflow      |
                   | (goreleaser -> GH Release     |
                   |  + brew formula PR            |
                   |  + npm publish + registry.json|
                   |  fingerprint update)          |
                   +---------------+---------------+
                                   |
            +----------------------+----------------------+
            v                      v                      v
     per-CLI GH Release     formula PR to tap      NPM @printing-press/<cli>
     with checksums         (e.g. espn-pp-cli.rb)  + platform optional deps
```

`ppl install <name>` flow:

```
ppl install espn
  -> read ~/.pp/taps/*/registry.json in priority order
  -> resolve "espn" to mvanhorn/printing-press-library entry
  -> look up latest release via entry.latest_release (or GitHub API fallback)
  -> pick archive for detected OS/arch from entry.bottles
  -> download archive + checksums.txt
  -> verify sha256 against entry.bottles[os-arch].sha256 AND checksums.txt
  -> extract espn-pp-cli binary to ~/.pp/bin/espn-pp-cli
  -> chmod +x, record install in ~/.pp/state.json
  -> print "installed espn-pp-cli vX.Y.Z"
```

Tap model: `~/.pp/taps/mvanhorn-printing-press-library/` is a git clone. `ppl tap update` fans out `git pull` across every tap dir. `registry.json` is the contract.

## Implementation Units

- [ ] **Unit 1: Release foundation for cli-printing-press (generator + MCP binaries only)**

**Goal:** Establish a working release pipeline for the generator repo that produces GitHub Release artifacts on tag push. This is the prerequisite for the tap repo, curl | sh installer, brew tap, and NPM pipeline. Scope: `printing-press` and `printing-press-mcp` only. The `ppl` binary ships from its own repo per KTD-16 and has its own Unit 1b.

**Requirements:** R4, R9

**Dependencies:** Unit 2 (the tap repo must exist before goreleaser can open a brew formula PR against it)

**Files:**
- Modify: `cli-printing-press/.goreleaser.yaml` (add `brews:` stanza for `printing-press` and `printing-press-mcp` targeting `mvanhorn/homebrew-printing-press`)
- Create: `cli-printing-press/.github/workflows/release.yml` (trigger on tag push, run goreleaser, mint a GitHub App installation token per-job; no long-lived PATs)
- Create: `cli-printing-press/.github/app.yml` (GitHub App binding for the tap-publish token)
- Create: `cli-printing-press/docs/releasing.md` (tag shape `printing-press/vX.Y.Z`; what the workflow does; how to roll back)

**Approach:**
- One release workflow keyed off tag prefix. Tag `printing-press/v*` triggers goreleaser for the generator; `printing-press-mcp/v*` triggers the MCP binary.
- Pre-flight: `goreleaser release --snapshot --skip=homebrew` runs first in a smoke-test job so non-brew paths are validated independently before the tap coupling activates.
- goreleaser publishes archives to GH Releases and opens a PR against `mvanhorn/homebrew-printing-press` for the formula. Formula PR is opt-in, not force-merge.
- Secret management uses `actions/create-github-app-token` to mint a short-lived installation token scoped to the tap repo only, per KTD-8 security review. No `HOMEBREW_TAP_TOKEN` PAT stored.
- Branch protection + 2FA on `mvanhorn` is a prerequisite, not a nice-to-have, and is checked into `docs/releasing.md`.

**Patterns to follow:**
- `printing-press-library/.github/workflows/generate-skills.yml` for the checkout + cache shape.
- Existing `cli-printing-press/.goreleaser.yaml` builds stanza for linux/darwin/windows x amd64/arm64.

**Test scenarios:**
- Happy path: push tag `printing-press/v0.0.1-rc1` -> workflow runs -> GH Release created with 6 archives + checksums.txt -> tap PR opens with `printing-press.rb`.
- Edge case: tag without slash (`v0.0.1`) -> workflow no-ops with a clear log line, does not release anything.
- Error path: `HOMEBREW_TAP_TOKEN` missing -> workflow fails at the brews step, GH Release still publishes (archives land even if tap PR fails).
- Integration: tap PR merged -> `brew tap mvanhorn/printing-press && brew install printing-press` works on macOS.

**Verification:**
- Tag `printing-press/v0.0.1-rc1` produces a GH Release with the expected archives and a tap PR within 10 minutes.
- `brew install printing-press` (after tap PR merge) installs a binary whose `printing-press version` matches the tag.

- [ ] **Unit 2: Create mvanhorn/homebrew-printing-press tap repo**

**Goal:** Stand up the tap repo so goreleaser has somewhere to push formulas. This unit precedes Unit 1 because the `brews:` stanza fails at runtime if the target repo does not exist.

**Requirements:** R1, R5

**Dependencies:** None (runs first)

**Files:**
- Create: `mvanhorn/homebrew-printing-press/README.md` (one-screen explainer; links back to printing-press-library)
- Create: `mvanhorn/homebrew-printing-press/.github/workflows/test.yml` (brew style + audit on PR)
- Create: `mvanhorn/homebrew-printing-press/Formula/.gitkeep` (empty dir for formulas)
- Modify across both source repos: every `.goreleaser.yaml` `brews.repository.owner` from `trevin-chow` to `mvanhorn`, `name` to `homebrew-printing-press`.

**Approach:**
- Repo starts empty. Formulas arrive as PRs from the release workflow.
- `brew test-bot` runs on PR to catch formula syntax errors and download-url changes.
- README documents the naming rule: formula file name is the binary name (e.g. `Formula/espn-pp-cli.rb`), not the entry `name` in registry.json.

**Patterns to follow:**
- Any existing Homebrew tap README (e.g. `homebrew/homebrew-cask`) for the shape. Keep ours short.

**Test scenarios:**
- Happy path: Unit 1's tap PR merges cleanly and `brew install` works for the generator.
- Edge case: two CLIs release the same hour -> two PRs queued, neither blocks the other.
- Error path: brew audit fails on a bad download URL -> PR cannot merge; release still has GH Release archives.
- Integration: `brew tap mvanhorn/printing-press` works and `brew search printing-press` surfaces formulas.

**Verification:**
- `brew tap-info mvanhorn/printing-press` returns the repo.
- `brew audit --strict --online <formula>` passes for the seed formula.

- [ ] **Unit 3: Create mvanhorn/ppl repo and ship `ppl` binary skeleton**

**Goal:** Ship a `ppl` binary with the subcommand surface in place, even if most commands are stubs. This is the thing users will call. Lives in its own repo per KTD-16.

**Requirements:** R2

**Dependencies:** Unit 2 (tap repo exists so `ppl` can talk to it); Unit 1 (shape to mirror for release workflow)

**Files:**
- Create: `mvanhorn/ppl/cmd/ppl/main.go` (cobra root; version from ldflags)
- Create: `mvanhorn/ppl/cmd/ppl/root.go`, `tap.go`, `doctor.go`, `find.go` (stubs for find; install/uninstall/upgrade stubs land here and are filled in by Unit 4)
- Create: `mvanhorn/ppl/internal/state/state.go` (reads/writes `~/.pp/pkg/state.json` per KTD-22)
- Create: `mvanhorn/ppl/internal/paths/paths.go` (carves `~/.pp/bin`, `~/.pp/pkg/{state.json,taps/,cache/}` per KTD-22; leaves `~/.pp/auth/` and `~/.pp/runtime/` untouched)
- Create: `mvanhorn/ppl/internal/tapsync/tapsync.go` (hardened git clone/pull per KTD-17; rejects non-github.com remotes in v1)
- Create: `mvanhorn/ppl/.goreleaser.yaml` (targets darwin/linux/windows x amd64/arm64; `brews:` pushes to `mvanhorn/homebrew-printing-press`)
- Create: `mvanhorn/ppl/.github/workflows/release.yml` (tag `ppl/v*` -> goreleaser; GitHub App token for tap PR)
- Create: `mvanhorn/ppl/README.md`, `mvanhorn/ppl/docs/releasing.md`
- Test: `mvanhorn/ppl/cmd/ppl/root_test.go`, `mvanhorn/ppl/internal/state/state_test.go`, `mvanhorn/ppl/internal/paths/paths_test.go`, `mvanhorn/ppl/internal/tapsync/tapsync_test.go`

**Approach:**
- Cobra root with stubs for install, uninstall, find, list, upgrade, info, doctor, tap. `find` is the package-catalog browse verb per KTD-23. `tap` and `doctor` ship working here; install/uninstall/upgrade are wired in Unit 4.
- State file is plain JSON, versioned, 0600 perms, lives at `~/.pp/pkg/state.json`.
- `ppl tap add mvanhorn/printing-press-library` uses `git clone --no-local --config protocol.file.allow=never --config core.hooksPath=/dev/null --config core.fsmonitor=false https://github.com/mvanhorn/printing-press-library ~/.pp/pkg/taps/mvanhorn-printing-press-library` per KTD-17.
- `ppl tap update` runs `git -c core.hooksPath=/dev/null -c core.fsmonitor=false pull --ff-only` across every tap dir.
- Tap remotes not under `https://github.com/` are rejected with exit 2 in v1.
- `ppl doctor` checks: `~/.pp/bin` appended to PATH (not prepended, per KTD-15), `git` binary available, at least one tap present, no tap older than 7 days, `~/.pp/` layout matches KTD-22, no binary in `~/.pp/bin` shadows a reserved system name.

**Patterns to follow:**
- `cli-printing-press/cmd/printing-press/` for Cobra wiring conventions (vendored via module, not copy-paste).
- `cli-printing-press/internal/` directory layout for internal packages.

**Test scenarios:**
- Happy path: fresh machine, `ppl tap add mvanhorn/printing-press-library` -> clones to `~/.pp/pkg/taps/`, updates state.json, exit 0.
- Happy path: `ppl tap list` after add -> shows one row with owner, repo, last-pulled timestamp.
- Edge case: `ppl tap add` with a URL that is not an owner/repo -> exit 2 with an actionable error.
- Edge case: same tap added twice -> second call is idempotent, warns, exit 0.
- Edge case: `ppl tap add` with a non-github.com URL (e.g. gitlab) -> exit 2 with "github-only in v1; request at <issue>".
- Edge case: existing `~/.pp/auth/` or `~/.pp/runtime/` directories (from Composio plan) present -> `ppl` operations do not touch or chmod those dirs.
- Error path: git not installed -> `ppl tap add` exits 5 with the command path that failed.
- Error path: GitHub unreachable -> exits 5 with the HTTP status.
- Error path: malicious tap with `.git/config` setting `core.sshCommand=touch /tmp/pwned` -> `ppl tap add` and `ppl tap update` both use `-c core.hooksPath=/dev/null` and do not execute; `/tmp/pwned` does not exist after the operation.
- Integration: `ppl doctor` after add -> reports PATH (appended not prepended), git version, 1 tap, 0 stale, KTD-22 layout green, no reserved-name collisions, overall green.
- Integration: `ppl doctor` flags PATH as yellow if `~/.pp/bin` is prepended rather than appended.
- Integration: state file round-trips through `state.Read(state.Write(s))`.

**Verification:**
- `ppl --help` lists all top-level verbs.
- `ppl tap add` + `ppl tap list` + `ppl doctor` work end-to-end against a real checkout of `printing-press-library`.
- State file is readable as JSON and has `version: 1` at the top.

- [ ] **Unit 4: `ppl install|uninstall|upgrade|info|list|find` against the tap**

**Goal:** Make the package manager actually manage packages. Read the tap's `registry.json` + `releases.json` (per KTD-20), resolve a CLI to a GitHub Release asset, download, verify, install to `~/.pp/bin/`. Note the verb rename: package-level browse is `ppl find`, not `ppl search`, per KTD-23.

**Requirements:** R1, R2, R3, R9

**Dependencies:** Unit 3 (state + paths + hardened tap clone), Unit 1 + Unit 2 (so `printing-press` itself has at least one GH Release to install as the first smoke test)

**Execution note:** Test-first. The install flow has too many seams (download, checksum, extract, PATH) to verify post-hoc; write integration tests with a fake tap + fake release server first.

**Files:**
- Create: `mvanhorn/ppl/cmd/ppl/install.go`, `uninstall.go`, `upgrade.go`, `info.go`, `list.go` (and fill in `find.go` from Unit 3)
- Create: `mvanhorn/ppl/internal/resolver/resolver.go` (registry + releases lookup, tap precedence, collision surfacing, trust-prompt gate per KTD-19)
- Create: `mvanhorn/ppl/internal/release/release.go` (GitHub Release API client, asset selection for OS/arch, checksum verify, cosign verification hook per KTD-18)
- Create: `mvanhorn/ppl/internal/install/install.go` (download, extract, move, chmod, record; reserved-name guard per KTD-15)
- Create: `mvanhorn/ppl/internal/trust/trust.go` (prompt + state-record for non-official taps per KTD-19)
- Test: corresponding `_test.go` per package. Include `testdata/` with a fake registry.json, releases.json, checksums.txt, and a cosign bundle.

**Approach:**
- Resolver reads every tap's `registry.json` (catalog) and `releases.json` (distribution) in priority order, official first per KTD-5. Collisions get surfaced with `ppl find <name>`.
- Non-official tap install triggers trust prompt per KTD-19 on first use; acknowledgement recorded in `state.json.trusted_taps[]`.
- Release client prefers `releases.json` entry for the CLI version; falls back to GitHub API `GET /repos/<owner>/<repo>/releases/latest` when the entry is missing (backward compat) and warns.
- Asset selection uses `runtime.GOOS` + `runtime.GOARCH`, maps to goreleaser archive name template. Fail cleanly with exit 3 when no asset matches (e.g. freebsd, linux-musl).
- Verification is layered per KTD-18: sha256 in releases.json MUST equal sha256 in GH Release checksums.txt MUST equal cosign-verified SLSA provenance (official tap, phase 2+). Any divergence is exit 5 with a "possible tampering" banner.
- Install writes archive to `~/.pp/pkg/cache/<sha>/`, extracts to a temp dir, checks binary name against the reserved-name list (`git`, `ssh`, `curl`, `wget`, `node`, `python`, `brew`, `sudo`, `systemctl`) and refuses with exit 2 on conflict, then atomically renames the target binary into `~/.pp/bin/`.

**Patterns to follow:**
- Go stdlib `archive/tar`, `archive/zip`, `crypto/sha256`.
- goreleaser archive naming conventions from Unit 1's `.goreleaser.yaml`.

**Test scenarios:**
- Happy path: `ppl install printing-press` on darwin_arm64 -> downloads the matching archive, verifies checksum, writes `~/.pp/bin/printing-press`, records in state, exit 0.
- Happy path: `ppl install espn --tap mvanhorn/printing-press-library` works even when a third-party tap also has `espn`.
- Edge case: `ppl install espn` when two taps define `espn` -> prefer official tap per KTD-5, print "using espn from mvanhorn/printing-press-library (another tap also provides it; use --tap to override)".
- Edge case: `ppl install unknown-cli` -> exit 3, suggest `ppl search`.
- Edge case: `ppl install espn` twice -> second call no-ops with "already installed" unless `--force`.
- Error path: download 404 -> exit 5 with URL attempted.
- Error path: sha256 mismatch -> exit 5, cache is deleted, no partial install.
- Error path: archive contains unexpected layout -> exit 5, temp dir cleaned up.
- Error path: unsupported OS/arch -> exit 3 with the list of supported targets.
- Integration: install -> `<binary> --version` prints the tag version.
- Integration: install -> uninstall -> state.json returns to empty; `~/.pp/bin/<binary>` removed; `~/.pp/auth/` and `~/.pp/runtime/` untouched.
- Integration: `ppl upgrade printing-press` detects a new tag, downloads, replaces binary atomically, preserves state.
- Integration: `ppl list` shows every installed package with version and tap source.
- Integration: `ppl info espn` shows registry entry, latest release, supported platforms, install status.
- Integration: install from a non-official tap triggers trust prompt; `--yes` or prior trust skips it; tampered binary still exits 5 even when trust is granted.
- Integration: attempted install of a binary named `git` (hypothetical malicious tap) exits 2 with "reserved binary name" error and the reserved list.
- Integration: SLSA provenance verification happens only when a cosign bundle is present (phase 2+); absence is a yellow doctor warning, not an install blocker in phase 1.

**Verification:**
- Full install -> uninstall -> reinstall cycle works against a local fake tap pointing at a local static file server.
- Against real `cli-printing-press` release from Unit 1: `ppl install printing-press` works on macOS arm64 and Linux amd64.

- [ ] **Unit 5: NPM publishing pipeline and `@printing-press/*` scope**

**Goal:** Every library CLI is installable via `npx @printing-press/<cli>` or `npm i -g @printing-press/<cli>`. This is the "OUR CLIs on NPM" axis.

**Requirements:** R1, R6, R7

**Dependencies:** Unit 1 (release pipeline shape to mirror), can run in parallel with Units 2-4

**Execution note:** Reserve the `@printing-press` NPM scope BEFORE merging this plan's PR, not as Unit 5's first task. Publish a placeholder `@printing-press/reserved@0.0.0-reserved` under a plan-owned account the same day the plan ships. This is a 30-second operation that closes the squatting window between plan publication and Unit 5 execution. If the scope is already taken when we check, fall back per KTD-7 and document the fallback scope in the same PR.

**Execution note (OSC reuse):** Before writing new code, port four patterns from `mvanhorn/open-source-contributor` so Unit 5 is not reinventing anything OSC already battle-tested:
- Port `skills/osc-status/handlers/handle-npm-failure.md`'s failure-classification logic verbatim into the NPM release workflow's failure handler; adjust the classifier for the scoped-binary shape (add classes for "platform dep not yet indexed", "meta published before platforms", "chmod lost execute bit"). The core matching-on-workflow-logs pattern is reusable as-is.
- Port the `npm view <pkg> version` version-exists guard from the same file into the publish step so a re-run of a bad build is a safe no-op, not a duplicate-version error.
- Wrap the publish workflow in the pre-push verification pattern from `skills/osc-status/handlers/pre-push-verification.md`: build + lint + test must pass locally before a tag push. Works unchanged because it is language-agnostic.
- Emit `=== CLI PACKAGE DETECTION GATE ===` and `=== RELEASE WORKFLOW PREFLIGHT ===` banners per KTD-25 before any merge or tag-push step. Gates live in the Claude Code skill that drives the release, not in the CI workflow.

**Files:**
- Create: `printing-press-library/tools/npm-publish/main.go` (generator: reads registry.json + releases.json, emits per-CLI `package.json` + stub bin script + platform optional deps)
- Create: `printing-press-library/tools/npm-publish/templates/` (templates for meta package and platform package)
- Create: `printing-press-library/.github/workflows/npm-release.yml` (triggered by `release.published` event from Unit 6; waits for GH Release archives; publishes to NPM via OIDC Trusted Publishing per KTD-24)
- Create: `printing-press-library/docs/npm-publishing.md` (esbuild pattern explainer, Trusted Publishing setup, how to recover from lockfile bug npm/cli#4828, recommend volta/fnm over system node)
- Modify: `printing-press-library/registry.json` (ship `npm_package` field in `releases.json` per KTD-20, not registry.json)

**Approach:**
- Meta package layout (generated): `package.json` with `bin`, `optionalDependencies` mapping every supported OS/arch combo to `@printing-press/<cli>-<os>-<arch>`, minimal stub `bin/<cli>.js`. The stub MUST be the smallest possible program: one `require.resolve` of the platform dep, one `execFile` with the original `process.argv`. No other `require()` calls in the meta package. Lint check in the generator enforces this per the security review (compromised meta package should not attack sibling globals).
- Stub resolution pattern copied from esbuild's `node-platform.ts`: `try { require.resolve('@printing-press/<cli>-' + platformArch) } catch { ... }`, then `path.join(dir, 'bin', 'cli' + (process.platform === 'win32' ? '.exe' : ''))`. This handles npm hoisted node_modules, pnpm symlinked `.pnpm/` store, and Yarn Berry PnP simultaneously.
- When the platform dep is missing (npm/cli#4828 lockfile bug), the stub prints a specific, actionable error: "platform binary `@printing-press/<cli>-<os>-<arch>` not found. This is a known npm bug: `rm -rf node_modules package-lock.json && npm install`. If the problem persists on musl/Alpine, see `docs/npm-publishing.md`." Exit 3.
- musl/Alpine detection: stub checks `process.report.getReport().header.glibcVersionRuntime` existence. Absence prints "musl libc detected; platform is unsupported in v1. Install via `ppl install <cli>` or `go install`." Exit 3.
- chmod verification before publish: goreleaser output binaries have the execute bit lost ~40% of the time on Linux runners. Workflow runs `chmod +x dist/<binary>` then verifies with `npm pack --dry-run | grep 755` BEFORE `npm publish`. Missing execute bit is a hard fail.
- Publish order is strict: every `@printing-press/<cli>-<os>-<arch>` platform package publishes first, workflow polls `npm view @printing-press/<cli>-<os>-<arch> version` until all N platforms are indexed (up to 120s), THEN the meta package publishes. Meta-first publication would leave a 30-120s window where `npx` gets "Cannot find module" errors.
- Credentials: OIDC Trusted Publishing per KTD-24; `id-token: write` in workflow permissions; no stored NPM token.
- Generator rebuilds package shells from the tap files on every release. Templates are directional guidance; the generator emits working JavaScript, not Go-templated JavaScript strings.
- Phase 1 publishes only CLIs in the official registry (not third-party taps). Third-party taps opt in via their own NPM story.
- Scope verification check: after reservation, Unit 5's first workflow step is `npm view @printing-press/espn` and fails if the reserved placeholder is no longer present (detects scope hijack).

**Patterns to follow:**
- esbuild's `npm/` directory layout: separate `esbuild` meta and `@esbuild/darwin-arm64` optional deps.
- `printing-press-library/.github/workflows/generate-skills.yml` for registry-driven generation on push.

**Test scenarios:**
- Happy path: `npx @printing-press/espn scores lakers` on darwin_arm64 -> NPM resolves `@printing-press/espn-darwin-arm64` only, stub bin execs binary, command runs.
- Happy path: `npm i -g @printing-press/espn` on linux_amd64 -> only `@printing-press/espn-linux-x64` is installed, binary on PATH as `espn-pp-cli`.
- Happy path: pnpm strict install (`node-linker=isolated`) with the meta package -> require.resolve finds the platform dep via the pnpm store symlink; binary runs.
- Happy path: Yarn Berry PnP install -> require.resolve honors PnP; binary runs.
- Edge case: alpine/musl detection -> stub prints the specific diagnostic with the `glibcVersionRuntime` absence and points to `docs/npm-publishing.md`; does not emit "Error loading shared library ld-linux-x86-64.so.2".
- Edge case: npm lockfile bug #4828 reproduction (cross-platform lockfile regen) -> stub detects the missing platform dep and prints the exact fix command; exit 3.
- Edge case: Windows x64 install -> stub appends `.exe` to the binary path; `process.platform === 'win32'` branch runs.
- Edge case: freshly published platform dep not yet indexed by NPM -> meta publish is gated on polling each platform dep's version; publish order is enforced.
- Error path: scope `@printing-press` reservation disappears -> verification check fails fast, workflow exits with "scope hijack" error and does not publish.
- Error path: binary in dist has mode 644 after goreleaser build -> chmod step fixes it; if chmod fails or `npm pack --dry-run` still shows non-executable, publish aborts.
- Error path: OIDC token not available (e.g. workflow not on allowed branch per Trusted Publishing config) -> publish fails before any bytes hit the registry.
- Error path: meta package attempts a `require()` of anything outside the platform dep -> generator lint check fails the PR.
- Integration: `npx @printing-press/espn --version` matches the release tag that triggered publication.
- Integration: generator output is idempotent: rerunning on the same tap files and tag produces byte-identical package shells.
- Integration: first CLI (espn) publishes end-to-end through the full platform-first + meta-last flow; the other 20 are queued behind a feature flag and ship in Unit 8.

**Verification:**
- `npm view @printing-press/espn versions` shows the released version within 5 minutes of tag push.
- `npx @printing-press/espn --version` works on macOS, Linux, Windows.
- Package size per platform dep is less than 15 MB (bounded check).

- [ ] **Unit 6: Per-CLI release workflow in printing-press-library**

**Goal:** Every library CLI releases on its own tag with GH Release archives, brew formula PR, and NPM trigger.

**Requirements:** R4, R5, R9

**Dependencies:** Units 1-5

**Files:**
- Create: `printing-press-library/.github/workflows/cli-release.yml` (trigger on tag matching `*/v*`; detect which CLI from the tag prefix; run that CLI's goreleaser)
- Modify: every `library/**/.goreleaser.yaml`: `brews.repository.owner` to `mvanhorn`, `name` to `homebrew-printing-press`; add GitHub App token minting step
- Create: `printing-press-library/releases.json` (new file per KTD-20; initial empty object; distribution metadata only)
- Modify: `printing-press-library/registry.json` (stays stable per KTD-20; no distribution fields added here)
- Create: `printing-press-library/tools/releases-update/main.go` (post-release job: reads GH Release metadata, append-only updates to `releases.json` keyed by CLI name, opens PR)
- Create: `printing-press-library/docs/releasing-a-cli.md` (how to release a single CLI: tag `<cli>/vX.Y.Z`, what the workflow does, which file gets updated)

**Approach:**
- Workflow parses `${GITHUB_REF_NAME}` as `<cli>/v<semver>`. Looks up `library/**/<cli>/.goreleaser.yaml`. Fails with a clear message if the CLI dir cannot be found.
- goreleaser runs per-CLI. Brew formula PR uses a short-lived GitHub App installation token scoped to the tap repo only.
- Post-release: `tools/releases-update` writes the new version, asset URLs, sha256 bottles, and npm package name into `releases.json` entry keyed by CLI name (not array position, per KTD-20). This ensures concurrent per-CLI releases do not merge-conflict on array positions. Updates are append-only within an entry's `versions[]`.
- A single "fan-in" cadence: three concurrent release PRs each update their own `releases.json` entry without collision; if two PRs touch the same entry's `versions[]`, the second retries after a `git pull --rebase`.
- NPM publish happens in a downstream workflow (Unit 5) that listens for the `release.published` event, gated on `releases.json` PR merge so `releases.json` and NPM stay in sync.
- Skill generator and megamcp continue reading `registry.json` for catalog data; `ppl` reads both files.
- OSC reuse: after tag push, the workflow schedules a post-push verification check using the pattern from `open-source-contributor/skills/osc-status/handlers/post-push-monitor.md`. The check runs at +9 minutes, polls `gh run view` for the release workflow, and reports to the release skill's `=== RELEASE VERIFICATION GATE ===` per KTD-25. Background-bash for in-session results; no CronCreate per institutional memory.

**Patterns to follow:**
- `printing-press-library/.github/workflows/generate-skills.yml` for registry-mutating automation.
- Unit 1's release workflow for the goreleaser shape.

**Test scenarios:**
- Happy path: tag `espn-pp-cli/v0.1.0` -> workflow identifies `library/media-and-entertainment/espn/`, runs its goreleaser, GH Release created, tap PR opened, NPM release triggered, registry-update PR opened.
- Edge case: tag that does not parse (`v0.1.0` alone, no slash) -> workflow no-ops with a clear log line.
- Edge case: tag for a CLI whose goreleaser does not exist -> fails fast before any publish step runs.
- Error path: goreleaser build fails -> GH Release not created, no tap PR, no NPM trigger, tag remains but no partial artifacts. Registry is unchanged.
- Error path: registry-update PR conflicts with a concurrent PR -> registry-update opens its PR anyway; human resolves conflict; install still works against GH Release directly (not blocked).
- Integration: across three CLIs released in the same hour, each produces an independent tap PR, NPM publish, and registry-update PR without interference.

**Verification:**
- Tag three CLIs back-to-back; all three produce GH Release + tap PR + NPM publish + registry PR within 10 minutes each.
- After merge of registry PRs, `ppl install <cli>` resolves from the local tap without an API round-trip.

- [ ] **Unit 7: curl | sh installer for `ppl`**

**Goal:** A single shell command installs `ppl`, adds `~/.pp/bin` to PATH guidance, and is auditable before running.

**Requirements:** R7

**Dependencies:** Unit 1 (needs GH Releases for the generator repo)

**Files:**
- Create: `mvanhorn/ppl/install.sh` (POSIX shell; lives in the ppl repo per KTD-16; shipped as a release asset, not served from `main`)
- Create: `mvanhorn/ppl/install.ps1` (PowerShell equivalent for Windows)
- Create: `mvanhorn/ppl/docs/install.md` (one-page human-readable install guide with the tagged release URL and verify commands)
- Modify: `printing-press-library/README.md` (top-of-fold install snippet pointing at the tagged release URL)

**Approach:**
- Install script is published as a GitHub Release asset attached to every `ppl/v*` tag, not served from a mutable branch. Canonical one-liner is `curl -fsSL https://github.com/mvanhorn/ppl/releases/download/ppl/vX.Y.Z/install.sh | sh`. Rationale (from security review): a compromised GitHub account could rewrite an `install.sh` on `main` and backdoor every pasted one-liner; tagged release assets are immutable per tag, and the `mvanhorn` account loses the ability to silently rewrite them.
- A detached minisign signature (`install.sh.minisig`) ships alongside every release asset; `docs/install.md` shows the verify command. A SLSA provenance attestation is attached via GitHub OIDC keyless signing (free). These are the phase-1 "signing" story pulled forward from phase 3 per KTD-18.
- Bash script: `set -eu`, `uname -s`, `uname -m`, map to goreleaser archive name, `curl -fsSL` the archive, `shasum -a 256` verify against the release's `checksums.txt` AND against an inline sha256 baked into the script at build time (defense in depth), extract to tmp, move to `~/.pp/bin/ppl`, chmod, append (not prepend) `~/.pp/bin` to PATH via `~/.zshrc` or `~/.bashrc` when `--add-to-path` is passed. Reject writing to system profiles (`/etc/*`) always.
- `--add-to-path` is idempotent: skip if the rc file already has a `~/.pp/bin` line.
- No sudo. No writes outside `~/.pp` and (with `--add-to-path`) the user's shell rc.
- Version pin: `PPL_VERSION` env var overrides latest; default resolves `latest` via GH Releases API.
- PowerShell script: same shape for Windows users; validates signature via `Get-FileHash` + minisign.

**Patterns to follow:**
- Homebrew's `install.sh` at `https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh` for structure (but we are much smaller because we only install one binary).
- rustup-init.sh for OS/arch detection idioms.

**Test scenarios:**
- Happy path: `curl -fsSL <url> | sh` on macOS arm64 -> installs ppl, prints PATH guidance, exit 0.
- Happy path: `PPL_VERSION=v0.0.2 curl -fsSL <url> | sh` -> installs that specific version.
- Edge case: `~/.pp/bin/ppl` already exists -> overwrites with a warning, preserves state.json.
- Edge case: unsupported OS/arch -> exit 3, print supported list.
- Error path: network down -> exit 5 with the URL that failed.
- Error path: checksum mismatch -> exit 5, tmp dir cleaned up, nothing installed.
- Integration: after install, `~/.pp/bin/ppl --version` runs and matches `PPL_VERSION`.
- Integration: script is idempotent; running twice produces the same state.

**Verification:**
- `sh install.sh` in a fresh Docker container (ubuntu-22.04) produces a working `ppl` binary.
- `curl <raw-url> | sh` on a fresh macOS install (intel and arm64) both succeed.

- [ ] **Unit 8: Seed release of all 21 library CLIs**

**Goal:** Fan Unit 6's pipeline across the full catalog so every CLI has GH Releases, brew formulas, and NPM packages on day one. This is the "catalog goes live" moment.

**Requirements:** R1, R4

**Dependencies:** Units 1-7 all green, registry fields populated for at least three CLIs without issue

**Files:**
- Modify: every `library/**/.goreleaser.yaml` (pin to `mvanhorn/homebrew-printing-press`, add `brews.install` block listing binary names explicitly so both CLI and MCP are installed)
- Create: `printing-press-library/docs/release-log.md` (append-only log of every released CLI tag, commit sha, who tagged, any issues)
- Modify: `printing-press-library/README.md` (add install table showing brew + npx + ppl commands per CLI)
- Modify: `printing-press-library/plugin/skills/ppl/SKILL.md` (add capability probe: call `ppl install --dry-run <name>` per KTD-12 and the architecture review; on any non-zero exit, fall back to `go install` path; this avoids silently stranding users on stale taps)

**Approach:**
- Release in waves of three. After each wave, run `ppl install` against every new release manually to catch pipeline bugs.
- Every wave release runs through the three KTD-25 structural gates (CLI PACKAGE DETECTION, RELEASE WORKFLOW PREFLIGHT, RELEASE VERIFICATION). OSC's 2026-03-30 incident proved that batch pressure is exactly when prose checks get skipped; 21 CLIs in waves of three is the highest-pressure moment in this plan. The gates are non-negotiable.
- First wave (smoke test): one CLI with no auth (archive-is), one with API key auth (linear), one with browser auth (instacart). Validates the easy, medium, and hard auth shapes.
- Middle waves: all registry entries sorted by last-modified date (newest first), so hotspots ship first.
- Final wave: the rest; include a CI gate that ensures every registry entry has a non-null `latest_release` before the phase closes.
- `/ppl` skill update is a small patch: step 0 in each install mode runs `which ppl`, branches on presence. Existing `go install` fallback stays verbatim.

**Patterns to follow:**
- Previous batch-PR rollouts (per-app-skills, per-app mcp servers) for staging and evidence capture per CLI.

**Test scenarios:**
- Happy path (per CLI): tag `<cli>/v<x>` -> GH Release + tap PR + NPM publish + registry PR -> `brew install`, `npx`, and `ppl install` all succeed for that CLI.
- Edge case: a CLI whose binary name differs from its registry name (dominos-pp-cli) -> workflow picks the correct archive, formula file is `dominos-pp-cli.rb`, NPM package is `@printing-press/dominos-pp-cli`.
- Edge case: a CLI with both `<cli>-pp-cli` and `<cli>-pp-mcp` binaries -> brew formula installs both into `bin/`, NPM ships both binaries in the platform package.
- Error path: a CLI fails goreleaser build (e.g. broken go.mod or compile error) -> other CLIs in the same wave are unaffected; that CLI blocks until fixed; release log records the failure.
- Integration: `/ppl install espn cli` on a machine with `ppl` on PATH -> skill delegates to `ppl install espn`, prints the same outcome as before.
- Integration: `/ppl install espn cli` on a machine without `ppl` -> skill falls back to `go install` path and still works.

**Verification:**
- `brew install mvanhorn/printing-press/<every-cli>` succeeds on one macOS machine and one Linux machine.
- `npx @printing-press/<every-cli> --version` succeeds on macOS, Linux, Windows.
- `ppl list` (after `ppl install <every-cli>`) shows every catalog entry with the right version.

- [ ] **Unit 9: Third-party tap polish, trust model, and revocation**

**Goal:** Third-party taps feel first-class but safe. `find` spans all taps, collisions are handled, `ppl doctor` surfaces stale or broken taps, non-official installs require explicit trust, and the official tap can publish a deny-list for known-bad taps.

**Requirements:** R3

**Dependencies:** Units 3, 4, 6

**Files:**
- Modify: `mvanhorn/ppl/internal/resolver/resolver.go` (multi-tap resolution with collision reporting; trust-gate per KTD-19; deny-list check)
- Modify: `mvanhorn/ppl/cmd/ppl/find.go` (display tap column; filter by `--tap`)
- Modify: `mvanhorn/ppl/cmd/ppl/tap.go` (add `ppl tap trust`, `ppl tap revoke` subcommands)
- Modify: `mvanhorn/ppl/cmd/ppl/doctor.go` (add tap-health checks: schema version, dead asset URLs for top 5 most-recent entries, last-pulled age, KTD-22 namespace layout, known-bad tap deny-list)
- Create: `mvanhorn/ppl/docs/taps.md` (how to publish a tap; tap-v1 schema contract per KTD-21; a minimal third-party tap example with zero `library/**` directories; signing and trust policy)
- Create: `mvanhorn/ppl/internal/tap/validate.go` (schema_version check, required-field check, URL validation, drops `path` from the tap contract per KTD-21)
- Modify: `printing-press-library/registry.json` (add top-level `revoked_taps: [{owner, repo, reason, since}]` per security review)

**Approach:**
- Tap entry schema in the validator is `{name, description, category, source: {type: "github_release", owner, repo}, artifact_name_template}` plus optional `mcp` per KTD-21. The `path` field is rejected in third-party taps and ignored with a warning in the official tap (only `tools/generate-skills` cares about it). Schema_version bumped to 2 only if a future change would be breaking.
- Official tap (mvanhorn/printing-press-library) always resolves first per KTD-5. Collisions print a warning on install unless `--tap` is passed.
- Non-official tap install triggers the KTD-19 trust prompt: tap URL, last-updated timestamp, release sha256, `[y/N]`. Acknowledgement is recorded in `state.json.trusted_taps[]` and scoped to that tap, not globally.
- `ppl tap trust <owner>/<repo>` and `ppl tap revoke <owner>/<repo>` manage the local trust list. `ppl install --from <tap> <name>` bypasses the official-first resolver for scripted use and implicitly trusts the listed tap for that invocation only.
- Official tap's `registry.json` carries a `revoked_taps[]` list (known-bad third-party taps). `ppl tap update` and `ppl install` refuse installs from any tap on the deny-list unless `--i-know-its-revoked` is passed. This closes the "third-party tap goes rogue, no recall mechanism" gap.
- `ppl doctor` warnings are yellow; errors are red. Yellow never blocks install; red blocks that specific tap's installs with an actionable message.
- Docs include a worked example of a minimal third-party tap repo: zero `library/**` directories, one CLI, binary hosted on a separate GitHub Release, artifact_name_template and sha256 in the tap's `releases.json`.

**Test scenarios:**
- Happy path: two taps, one with `espn`, one without collisions -> `ppl install espn` from official, `ppl install some-third-party-cli` from third-party triggers the trust prompt on first use and installs on `y`.
- Happy path: `ppl find sports` spans both taps and shows a tap column.
- Happy path: `ppl tap trust other/tap` pre-approves; subsequent `ppl install` from that tap skips the prompt.
- Edge case: third-party tap registry has `schema_version: 2` (future) -> validator refuses with a clear version-upgrade message.
- Edge case: collision `espn` in both taps -> install prefers official, prints "also available in other/tap; use --from other/tap to override".
- Edge case: third-party tap with a `path` field -> validator rejects with "path is not part of the tap-v1 contract".
- Edge case: tap on the official revoked_taps list -> `ppl install` from that tap refuses with "revoked: <reason>; override with --i-know-its-revoked at your own risk".
- Error path: third-party tap with malformed `registry.json` -> `doctor` flags it red; `install` from that tap fails but other taps still work.
- Error path: dead asset URL in a tap entry -> `doctor` reports it; `install` from that entry exits 5 before download.
- Error path: `ppl tap add non-github.com/repo` -> exit 2, github-only in v1.
- Integration: full cycle: `ppl tap add` a test tap -> trust prompt on first install -> `ppl find` sees its CLIs with tap column -> `ppl install` succeeds -> `ppl doctor` reports green.
- Integration: revoking a previously trusted tap with `ppl tap revoke` removes it from `trusted_taps[]` and re-prompts on next install.

**Verification:**
- Adding a local filesystem-backed test tap with two CLIs and no collisions works end-to-end.
- Collision with the official tap is surfaced per KTD-5.

## System-Wide Impact

- **Interaction graph:** The `/ppl` skill (printing-press-library plugin) now conditionally delegates to the `ppl` binary via `ppl install --dry-run` capability probe. Generator workflows in `cli-printing-press` are untouched in behavior but gain GitHub App token wiring. Per-CLI goreleaser configs are modified in lockstep to point at `mvanhorn/homebrew-printing-press`. Catalog is split per KTD-20: `registry.json` (stable, read by skill generator + megamcp + `ppl`) and new `releases.json` (volatile, read by `ppl` only). A new `mvanhorn/ppl` repo ships the package-manager binary independently from the generator.
- **Error propagation:** Every failure mode in install (network, checksum, archive, PATH) must exit with a typed code and include the URL, file, or path that failed so agents can self-correct. `ppl doctor` is the canonical place where errors become actionable advice.
- **State lifecycle risks:** `~/.pp/pkg/state.json` is a single JSON file written atomically (write-to-tmp-then-rename). Partial downloads land in `~/.pp/pkg/cache/<sha>/` and are deleted on verify failure. Uninstall must not leave orphan `state.json` entries (state write happens last in install, first in uninstall). `~/.pp/auth/` and `~/.pp/runtime/` are owned by the sibling Composio runtime per KTD-22; `ppl` operations never chmod, delete, or walk those dirs.
- **API surface parity:** `ppl install` and `brew install` and `npx @printing-press/<cli>` must all install the same binary at the same version for a given tag. If they diverge, registry.json is wrong and `ppl doctor` should detect it.
- **Integration coverage:** Every install channel (brew, npm, ppl, curl | sh) needs one real end-to-end test per phase. Unit tests with fake registries are necessary but not sufficient.
- **Unchanged invariants:** Existing `go install` paths for both the generator and every library CLI keep working identically. Existing `/plugin install` works. Existing env-var-based auth in every printed CLI works. None of that is touched.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| `@printing-press` NPM scope already taken or squatted after plan publication | Reserve the scope with a placeholder `@printing-press/reserved@0.0.0-reserved` BEFORE the plan PR merges per Unit 5's execution note. Fall back to `@pp-library` or `@ppcli` per KTD-7 only if genuinely unavailable. |
| Malicious `.git/config` or hooks in a third-party tap -> RCE on `git pull` | All tap clones use `--no-local --config protocol.file.allow=never --config core.hooksPath=/dev/null --config core.fsmonitor=false` per KTD-17; all pulls run with `-c core.hooksPath=/dev/null -c core.fsmonitor=false`; non-github.com remotes are rejected in v1. |
| Compromised `mvanhorn` GitHub account rewrites `install.sh` on `main` | `install.sh` is served as a tagged GitHub Release asset (immutable per tag) with a minisign detached signature and a SLSA provenance attestation per KTD-18; README one-liner points at the tagged URL. 2FA + branch protection on `mvanhorn` is a documented prerequisite. |
| Release workflow secret token leak | `HOMEBREW_TAP_TOKEN` is replaced by a short-lived GitHub App installation token minted per-job per KTD-8 security review. NPM uses OIDC Trusted Publishing per KTD-24; no stored NPM token. |
| Third-party tap publishes an unsigned malicious binary | (a) Official tap resolves first per KTD-5; (b) non-official installs require explicit trust prompt per KTD-19; (c) binary names matching a reserved system list (`git`, `ssh`, `curl`, etc.) are rejected per KTD-15; (d) official tap publishes a revoked_taps deny-list per Unit 9. |
| Known-bad third-party tap, no recall mechanism | Official `registry.json` carries `revoked_taps[]`; `ppl tap update` and `ppl install` refuse revoked taps unless the user overrides with `--i-know-its-revoked`. |
| Trust root for checksums collapses if `mvanhorn` is compromised | Layered verification per KTD-18: sha256 in releases.json = sha256 in GH Release checksums.txt = cosign-verified SLSA provenance. Any divergence is exit 5. |
| macOS Gatekeeper prompts scare off users | `ppl doctor` prints the verification steps (sha256 compare, tag URL, minisign verify) first; only prints the `xattr -d com.apple.quarantine` recipe as a last step, not the default. Revisit Apple Developer ID + notarization in phase 3 based on install numbers. |
| Homebrew tap review turnaround slows down release cadence | Tap PRs merge on a rolling basis via `brew test-bot` auto-approval on clean audits; a human only reviews when audit fails. |
| Platform coverage gaps (alpine/musl, freebsd, linux/arm other than arm64) | v1 supports darwin/linux/windows on amd64/arm64 only; stub bin detects musl via `glibcVersionRuntime` absence and prints a specific diagnostic; installer prints a clean unsupported-platform message with the supported list. |
| `~/.pp/bin` not on PATH -> silent install failure | `ppl doctor` runs at install end; `install.sh` and `ppl install <first-time>` both print PATH guidance; `--add-to-path` appends (not prepends) to `~/.zshrc` or `~/.bashrc`; never writes to `/etc/*`. |
| `~/.pp/bin` prepended to PATH shadows system binaries | KTD-15 enforces append-only; `ppl doctor` warns yellow if the shell rc prepends; `ppl install` rejects binary names on a reserved system list (`git`, `ssh`, `curl`, `wget`, `node`, `python`, `brew`, `sudo`, `systemctl`). |
| `~/.pp/` namespace collision with sibling Composio runtime | KTD-22 carves the namespace: `~/.pp/bin/` shared, `~/.pp/pkg/` owned by ppl, `~/.pp/auth/` + `~/.pp/runtime/` owned by Composio runtime. Both doctors check the layout. |
| Tap staleness leads to downloading removed releases | `ppl install` validates against GH Releases API even when releases.json says a version exists; `ppl doctor` warns on tap older than 7 days. |
| Release workflow races on `releases.json` | Entries are keyed by CLI name (not array position) per KTD-20; concurrent release PRs touch disjoint entries; in-entry `versions[]` conflicts are handled by rebase-and-retry. |
| Breaking change to `registry.json` or `releases.json` shape breaks `ppl` in the wild | `schema_version` enforced in Unit 9 validator per KTD-20; any breaking change bumps major version and is refused by older clients with an actionable upgrade message. |
| `/ppl` skill silently strands users when `ppl` is installed but tap is stale | Skill probes with `ppl install --dry-run <name>` per KTD-12 and the architecture review; any non-zero exit falls back to `go install`. |
| NPM lockfile bug #4828 drops platform optional deps on cross-platform lockfile regen | Stub bin detects the missing platform dep and prints the exact fix command (`rm -rf node_modules package-lock.json && npm install`); documented in `docs/npm-publishing.md`. |
| NPM meta package compromise attacks sibling globals | Unit 5 bin stub is minimal: one `require.resolve` + `execFile`; generator lint check refuses any other `require()` in the meta package. Docs recommend volta/fnm over system node for global installs. |
| Binary execute bit lost on Linux CI runners during NPM publish | Unit 5 workflow runs `chmod +x` before publish and verifies with `npm pack --dry-run`; missing execute bit is a hard-fail before `npm publish` runs. |
| NPM meta package published before platform deps are indexed | Unit 5 publish order is strict: platform deps first, poll each with `npm view` until indexed (up to 120s), THEN meta; a meta-first window would produce "Cannot find module" errors in `npx`. |
| Release workflow races produce duplicate formula PRs or NPM versions | Per-CLI tags serialize naturally; goreleaser deduplicates formula names; NPM publish is idempotent per version. |
| `ppl search` collides semantically with `printing-press run search` | Renamed to `ppl find` per KTD-23; `/ppl` skill help text cross-links both verbs. |
| Under batch pressure (21-CLI fan-out in Unit 8), prose release checks get skipped and a CLI publishes broken | Three mandatory structural gates per KTD-25 replace prose instructions at detection, preflight, and verification. OSC hit this exact bug on 2026-03-30. |
| Rebuilding OSC's NPM-failure recovery and post-push monitoring from scratch wastes weeks | OSC reuse audit completed; four patterns (failure classification, version-exists guard, pre-push verification, post-push monitor) port directly into Units 5 and 6 per Institutional Learnings. |

## Documentation / Operational Notes

- `printing-press-library/docs/install.md` is the canonical user-facing install guide. Linked from top of README.
- `printing-press-library/docs/taps.md` documents the tap protocol.
- `cli-printing-press/docs/releasing.md` and `printing-press-library/docs/releasing-a-cli.md` document the tag-push release flow.
- Release log at `printing-press-library/docs/release-log.md` is append-only and captures any post-release incidents.
- Rollout order: Units 1 -> 2 -> 3 -> 4 (with one smoke-test CLI released in Unit 4 end-to-end) -> 5 -> 6 -> 7 -> 8 (catalog fan-out) -> 9. Ship phase 1 when `brew install printing-press` works; ship phase 2 when one CLI is installable through all four channels; ship phase 3 when the full catalog is live; ship phase 4 (Unit 9) as polish.
- Rollback plan: every install channel is a pull from GH Releases. If a release is bad, delete the release + tag and publish an advisory in `release-log.md`. `ppl upgrade` will then no-op on the bad version. NPM can `npm deprecate` a version without unpublishing.
- Monitoring: no install-time telemetry in v1. Watch GitHub Releases download counts, NPM weekly downloads, and brew analytics (opt-in) as the leading indicators.

## Phased Delivery

- **Phase 0 (pre-plan-merge): Scope reservation.** Reserve `@printing-press` on NPM with a placeholder package before this plan's PR merges. Closes the squatting window that opens the moment the plan becomes public. 30 seconds.
- **Phase 1: Release foundation (Units 2, 1).** Tap repo exists (Unit 2 runs first); the generator repo releases real binaries on tag push with GitHub App tokens and SLSA provenance. Unblocks everything else. Ships when `brew install mvanhorn/printing-press/printing-press` works.
- **Phase 2: `ppl` binary + one CLI end-to-end (Units 3, 4, 6 for one CLI).** The `mvanhorn/ppl` repo exists, the binary ships, it can install from the official tap with cosign verification against one real library CLI release. Ships when `ppl install espn` works and `ppl doctor` reports green.
- **Phase 3: NPM + curl | sh + catalog (Units 5, 7, 8).** Every library CLI is installable through every channel; NPM publishes via OIDC; install.sh is signed and served from tagged releases; the Claude Code skill delegates via `--dry-run` probe. Ships when the install table in the README is filled in for every row.
- **Phase 4: Ecosystem polish (Unit 9).** Third-party taps feel first-class, trust model is enforced, revocation works. Ships when a test third-party tap can be added, its install triggers the trust prompt, and the revoked_taps deny-list blocks a revoked tap end-to-end.

## Sources & References

- `cli-printing-press/.goreleaser.yaml`
- `printing-press-library/library/media-and-entertainment/espn/.goreleaser.yaml` (representative per-CLI goreleaser)
- `printing-press-library/registry.json` (catalog schema)
- `printing-press-library/plugin/skills/ppl/SKILL.md` (today's router; delegation target)
- `cli-printing-press/docs/plans/2026-04-19-001-feat-composio-inspired-features-plan.md` (sibling plan; runtime super-CLI under `printing-press run`; does not conflict with this plan's `ppl` binary)
- `printing-press-library/docs/plans/2026-04-10-002-feat-claude-code-plugin-plan.md` (plugin plumbing precedent)
- External: esbuild NPM layout and `node-platform.ts` reference, rustup-init.sh, Homebrew tap conventions
- External: npm Trusted Publishing with OIDC (GA July 2025), npm/cli#4828 + #8320 lockfile bugs, openai/codex#9520 + microsoft/node-pty#850 execute-bit bugs
- External: cosign keyless signing via GitHub OIDC, SLSA provenance attestation, minisign
- Institutional memory: always PR before main; npm availability check before merge; /ppl lives in public repo only

## Deepening Summary (2026-04-19)

Deepening pass added 9 new KTDs (KTD-16 through KTD-24), restructured Units 1-9, and expanded the Risks table from 10 rows to 22 rows. The pass was driven by three parallel expert reviews:

- **Security review** surfaced seven issues ranging from git-hook RCE in tap clones (critical) to PATH ordering as a local privilege path (medium). Changes: KTD-17 (hardened tap clones), KTD-18 (explicit trust root with cosign/SLSA moved to phase 2), KTD-19 (non-official tap trust prompt), KTD-24 (OIDC Trusted Publishing instead of NPM_TOKEN), KTD-15 updated for append-only PATH, Unit 7 restructured to serve install.sh from tagged releases with minisign.
- **Architecture review** surfaced the KTD-2 repo choice error (ppl should not live in cli-printing-press), the `registry.json` God-file drift, the collision between `ppl search` and `printing-press run search`, the Unit 1/Unit 2 circular dependency, and the `~/.pp/` namespace collision with the sibling Composio runtime. Changes: KTD-16 (new ppl repo), KTD-20 (split registry.json + releases.json), KTD-21 (tap-v1 schema drops `path`), KTD-22 (shared namespace carveout), KTD-23 (rename `ppl search` to `ppl find`), unit reordering so Unit 2 precedes Unit 1.
- **NPM best-practices review** surfaced five operational pitfalls: lockfile bug #4828, execute-bit loss on Linux CI, meta-first publish gap, musl detection silently failing, and the Classic Token deprecation. Changes woven into Unit 5 (OIDC, chmod verification, staged publish order, explicit musl diagnostic, scope reservation as phase 0).
- **OSC NPM tooling reuse audit** (added 2026-04-19 after initial deepening) surveyed the `mvanhorn/open-source-contributor` handlers and plans for patterns that transfer to this plan's NPM and release-verification work. Four patterns port directly (failure classification, version-exists guard, pre-push verification, post-push monitor). The npm-specific version-bump choreography does not (we use Git tags, not `package.json`). One hard-won OSC lesson shipped as KTD-25: release-flow decision points are mandatory structural gates, not prose. OSC learned this from their 2026-03-30 npm-detection incident.

No product requirements changed; no scope was added. The plan is materially more defensible and the number of foot-guns in the happy path is meaningfully lower.
