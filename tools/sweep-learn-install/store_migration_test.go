package main

import (
	"strings"
	"testing"
)

// preLearnStoreSnippet is a minimal store.go fragment carrying the
// post-U6 learn-migrations anchor. The sweep finds the anchor and
// rewrites the block between it and the FTS create.
const preLearnStoreSnippet = `package store

const StoreSchemaVersion = 1

func migrate() {
	migrations := []string{
		` + "`CREATE TABLE IF NOT EXISTS resources (id TEXT)`" + `,
		// CLI Printing Press: learn migrations
		` + "`CREATE TABLE IF NOT EXISTS search_learnings (old shape)`" + `,
		` + "`CREATE VIRTUAL TABLE IF NOT EXISTS search_learnings_fts USING fts5(query_pattern, tokenize='porter unicode61')`" + `,
	}
	_ = migrations
}
`

// preLearnNoAnchorSnippet is the shape every CLI generated before U6
// carries. The migrations slice is canonical-templated, but the anchor
// comment is missing entirely. Bootstrap mode seeds the anchor + the
// learn-migrations block, and a subsequent re-run lands in anchor mode
// with zero diff.
const preLearnNoAnchorSnippet = `package store

const StoreSchemaVersion = 1

func migrate() {
	migrations := []string{
		` + "`CREATE TABLE IF NOT EXISTS resources (id TEXT)`" + `,
	}
	_ = migrations
}
`

// nonTemplatedStoreSnippet is a hand-modified store.go shape the
// bootstrap path must refuse: the migrations identifier was renamed,
// so AST detection cannot locate the canonical slice and the splice
// would be ambiguous.
const nonTemplatedStoreSnippet = `package store

const StoreSchemaVersion = 1

func migrate() {
	customMigrations := []string{
		` + "`CREATE TABLE IF NOT EXISTS resources (id TEXT)`" + `,
	}
	_ = customMigrations
}
`

// multiMigrationsStoreSnippet declares two `migrations := []string{...}`
// slices. Bootstrap mode refuses on this shape so we never splice into
// the wrong one.
const multiMigrationsStoreSnippet = `package store

const StoreSchemaVersion = 1

func migrate() {
	migrations := []string{
		` + "`CREATE TABLE IF NOT EXISTS resources (id TEXT)`" + `,
	}
	_ = migrations
}

func migrateExtra() {
	migrations := []string{
		` + "`CREATE TABLE IF NOT EXISTS extra (id TEXT)`" + `,
	}
	_ = migrations
}
`

// preU6NoVersionSnippet has the canonical migrations slice but no
// `const StoreSchemaVersion` declaration. Bootstrap must seed both the
// learn-migrations block AND the version constant.
const preU6NoVersionSnippet = `package store

func migrate() {
	migrations := []string{
		` + "`CREATE TABLE IF NOT EXISTS resources (id TEXT)`" + `,
	}
	_ = migrations
}
`

func TestHasLearnMigrationAnchor(t *testing.T) {
	if !hasLearnMigrationAnchor([]byte(preLearnStoreSnippet)) {
		t.Error("expected anchor to be detected in pre-learn snippet")
	}
	if hasLearnMigrationAnchor([]byte(preLearnNoAnchorSnippet)) {
		t.Error("expected anchor absent in no-anchor snippet")
	}
}

func TestPatchStoreMigrations_RewritesBlockAndBumpsVersion(t *testing.T) {
	ctx := sweepCtx{CLIName: "demo-pp-cli", APIName: "demo"}
	got, changed, err := patchStoreMigrations(preLearnStoreSnippet, ctx)
	if err != nil {
		t.Fatalf("patchStoreMigrations: %v", err)
	}
	if !changed {
		t.Error("expected changed=true on first run")
	}
	if !strings.Contains(got, "search_patterns") {
		t.Errorf("canonical block missing search_patterns:\n%s", got)
	}
	if !strings.Contains(got, "entity_lookups") {
		t.Errorf("canonical block missing entity_lookups:\n%s", got)
	}
	if !strings.Contains(got, "teach_log_metadata") {
		t.Errorf("canonical block missing teach_log_metadata:\n%s", got)
	}
	if strings.Contains(got, "old shape") {
		t.Errorf("stale (old shape) content was not replaced:\n%s", got)
	}
	if !strings.Contains(got, "const StoreSchemaVersion = 3") {
		t.Errorf("StoreSchemaVersion not bumped to 3:\n%s", got)
	}
}

func TestPatchStoreMigrations_Idempotent(t *testing.T) {
	ctx := sweepCtx{CLIName: "demo-pp-cli", APIName: "demo"}
	first, _, err := patchStoreMigrations(preLearnStoreSnippet, ctx)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, changed, err := patchStoreMigrations(first, ctx)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if changed {
		t.Error("expected changed=false on idempotent re-run")
	}
	if second != first {
		t.Errorf("second run produced diff:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// TestAnchorBootstrap_AddsAnchorAndMigrationsWhenMissing asserts the
// bootstrap path seeds the anchor + the canonical block into a pre-U6
// store.go. After bootstrap the file carries the anchor, the 5 learn
// tables, and the bumped StoreSchemaVersion.
func TestAnchorBootstrap_AddsAnchorAndMigrationsWhenMissing(t *testing.T) {
	ctx := sweepCtx{CLIName: "demo-pp-cli", APIName: "demo"}
	got, changed, err := patchStoreMigrations(preLearnNoAnchorSnippet, ctx)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if !changed {
		t.Error("expected changed=true on bootstrap")
	}
	if !strings.Contains(got, learnMigrationAnchor) {
		t.Errorf("anchor not present after bootstrap:\n%s", got)
	}
	if !strings.Contains(got, "search_learnings") {
		t.Errorf("search_learnings CREATE missing after bootstrap:\n%s", got)
	}
	if !strings.Contains(got, "search_patterns") {
		t.Errorf("search_patterns CREATE missing after bootstrap:\n%s", got)
	}
	if !strings.Contains(got, "entity_lookups") {
		t.Errorf("entity_lookups CREATE missing after bootstrap:\n%s", got)
	}
	if !strings.Contains(got, "teach_log_metadata") {
		t.Errorf("teach_log_metadata CREATE missing after bootstrap:\n%s", got)
	}
	if !strings.Contains(got, learnMigrationsBlockEndMarker) {
		t.Errorf("FTS create missing after bootstrap:\n%s", got)
	}
	if !strings.Contains(got, "const StoreSchemaVersion = 3") {
		t.Errorf("StoreSchemaVersion not bumped to 3 by bootstrap:\n%s", got)
	}
	// The pre-existing `resources` CREATE survives untouched — bootstrap
	// inserts, it doesn't rewrite.
	if !strings.Contains(got, "CREATE TABLE IF NOT EXISTS resources") {
		t.Errorf("pre-existing resources CREATE dropped by bootstrap:\n%s", got)
	}
}

// TestAnchorBootstrap_Idempotent asserts a second run on bootstrap-
// emitted source produces zero diff. The second run lands in the
// anchor path (the bootstrap output carries the anchor) and re-emits
// the same canonical block byte-for-byte.
func TestAnchorBootstrap_Idempotent(t *testing.T) {
	ctx := sweepCtx{CLIName: "demo-pp-cli", APIName: "demo"}
	first, _, err := patchStoreMigrations(preLearnNoAnchorSnippet, ctx)
	if err != nil {
		t.Fatalf("first (bootstrap): %v", err)
	}
	second, changed, err := patchStoreMigrations(first, ctx)
	if err != nil {
		t.Fatalf("second (anchor path): %v", err)
	}
	if changed {
		t.Error("expected changed=false on idempotent re-run after bootstrap")
	}
	if second != first {
		t.Errorf("second run produced diff after bootstrap:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// TestAnchorBootstrap_RefusesNonTemplatedStore asserts the bootstrap
// path refuses to splice when the migrations identifier doesn't match
// the canonical name. A renamed identifier is the most common shape
// of a hand-modified store.go and must surface a clear "manual review"
// error rather than a silent partial splice.
func TestAnchorBootstrap_RefusesNonTemplatedStore(t *testing.T) {
	ctx := sweepCtx{CLIName: "demo-pp-cli", APIName: "demo"}
	_, _, err := patchStoreMigrations(nonTemplatedStoreSnippet, ctx)
	if err == nil {
		t.Fatal("expected error on non-templated store.go")
	}
	if !strings.Contains(err.Error(), "manual review") {
		t.Errorf("expected manual-review diagnostic; got %v", err)
	}
}

// TestAnchorBootstrap_RefusesMultipleMigrationsSlices asserts the
// bootstrap path refuses when the file declares more than one
// `migrations := []string{...}` slice. The splice site would otherwise
// be ambiguous.
func TestAnchorBootstrap_RefusesMultipleMigrationsSlices(t *testing.T) {
	ctx := sweepCtx{CLIName: "demo-pp-cli", APIName: "demo"}
	_, _, err := patchStoreMigrations(multiMigrationsStoreSnippet, ctx)
	if err == nil {
		t.Fatal("expected error on multiple migrations slices")
	}
	if !strings.Contains(err.Error(), "manual review") {
		t.Errorf("expected manual-review diagnostic; got %v", err)
	}
}

// TestAnchorBootstrap_SeedsStoreSchemaVersionWhenAbsent asserts a
// pre-U6 store.go that never carried a StoreSchemaVersion declaration
// receives one alongside the bootstrap-inserted anchor.
func TestAnchorBootstrap_SeedsStoreSchemaVersionWhenAbsent(t *testing.T) {
	ctx := sweepCtx{CLIName: "demo-pp-cli", APIName: "demo"}
	got, changed, err := patchStoreMigrations(preU6NoVersionSnippet, ctx)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if !changed {
		t.Error("expected changed=true on bootstrap of pre-U6 store.go")
	}
	if !strings.Contains(got, "const StoreSchemaVersion = 3") {
		t.Errorf("StoreSchemaVersion not seeded by bootstrap:\n%s", got)
	}
}

func TestBumpStoreSchemaVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"bumps-lower-version",
			"const StoreSchemaVersion = 1",
			"const StoreSchemaVersion = 3",
		},
		{
			"idempotent-at-target",
			"const StoreSchemaVersion = 3",
			"const StoreSchemaVersion = 3",
		},
		{
			"leaves-higher-alone",
			"const StoreSchemaVersion = 5",
			"const StoreSchemaVersion = 5",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := bumpStoreSchemaVersion(tc.in, 3)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
