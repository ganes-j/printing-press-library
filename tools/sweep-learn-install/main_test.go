package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSweepCLI_SkipsMissingManifest exercises skip rule #1: a directory
// without .printing-press.json is skipped silently.
func TestSweepCLI_SkipsMissingManifest(t *testing.T) {
	dir := t.TempDir()
	status, err := sweepCLI(dir, sweepOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != statusSkipped {
		t.Errorf("expected statusSkipped; got %q", status)
	}
}

// TestSweepCLI_SkipsOptOutMarker exercises skip rule #2: a directory
// with .no-learn-sweep is skipped.
func TestSweepCLI_SkipsOptOutMarker(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".printing-press.json"),
		`{"api_name":"demo","cli_name":"demo-pp-cli"}`)
	writeFile(t, filepath.Join(dir, ".no-learn-sweep"), "")
	status, err := sweepCLI(dir, sweepOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != statusSkipped {
		t.Errorf("expected statusSkipped (opt-out); got %q", status)
	}
}

// TestSweepCLI_RefusesLegacyRootShape exercises skip rule #3: a CLI
// with the legacy `var rootCmd` shape is refused.
func TestSweepCLI_RefusesLegacyRootShape(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".printing-press.json"),
		`{"api_name":"demo","cli_name":"demo-pp-cli"}`)
	writeFile(t, filepath.Join(dir, "internal/cli/root.go"), legacyRootShape)

	status, err := sweepCLI(dir, sweepOpts{})
	if err == nil {
		t.Fatal("expected error for legacy shape")
	}
	if status != statusSkipped {
		t.Errorf("expected statusSkipped; got %q", status)
	}
	if !strings.Contains(err.Error(), "legacy var rootCmd shape") {
		t.Errorf("expected legacy-shape diagnostic; got %v", err)
	}
}

// TestSweepCLI_SkipsNonTemplatedStore exercises skip rule #4: a CLI
// whose store.go has been hand-edited so the canonical
// `migrations := []string{...}` slice can't be located is refused with
// a manual-review diagnostic. (The post-U6 anchor-mode path and the
// bootstrap path both handle the templated shapes; this case is the
// out-of-contract drift.)
func TestSweepCLI_SkipsNonTemplatedStore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".printing-press.json"),
		`{"api_name":"demo","cli_name":"demo-pp-cli"}`)
	writeFile(t, filepath.Join(dir, "internal/cli/root.go"), canonicalRootFlagsShape)
	writeFile(t, filepath.Join(dir, "internal/store/store.go"), nonTemplatedStoreSnippet)

	status, err := sweepCLI(dir, sweepOpts{})
	if err == nil {
		t.Fatal("expected error for non-templated store.go")
	}
	if status != statusSkipped {
		t.Errorf("expected statusSkipped; got %q", status)
	}
	if !strings.Contains(err.Error(), "manual review") {
		t.Errorf("expected manual-review diagnostic; got %v", err)
	}
}

// TestSweepCLI_IdempotentOnSecondRun runs the full sweep twice on the
// same fixture and asserts the second run reports statusUnchanged.
// This is the binding idempotency contract for the per-CLI pipeline.
func TestSweepCLI_IdempotentOnSecondRun(t *testing.T) {
	dir := stageMinimalCLIDir(t)

	// First run: should patch.
	st1, err := sweepCLI(dir, sweepOpts{})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if st1 != statusPatched {
		t.Errorf("first run expected patched; got %q", st1)
	}

	// Second run: should report unchanged.
	st2, err := sweepCLI(dir, sweepOpts{})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if st2 != statusUnchanged {
		t.Errorf("second run expected unchanged; got %q", st2)
	}
}

// TestSweepCLI_DryRunDoesNotWrite verifies the -dry-run flag short-
// circuits before any file write. The pre-sweep manifest survives
// untouched.
func TestSweepCLI_DryRunDoesNotWrite(t *testing.T) {
	dir := stageMinimalCLIDir(t)
	manifestPath := filepath.Join(dir, ".printing-press.json")
	before, _ := os.ReadFile(manifestPath)

	if _, err := sweepCLI(dir, sweepOpts{DryRun: true}); err != nil {
		t.Fatalf("dry-run: %v", err)
	}

	after, _ := os.ReadFile(manifestPath)
	if string(before) != string(after) {
		t.Errorf("dry-run wrote to manifest: %s -> %s", before, after)
	}
	// Learn files should not appear.
	if _, err := os.Stat(filepath.Join(dir, "internal/learn/recall.go")); err == nil {
		t.Error("dry-run wrote learn files; expected none")
	}
}

// TestSweepCLI_ReadmeOnlyOnlyTouchesSkill verifies the -readme-only
// branch skips Go-source surgery and writes nothing else.
func TestSweepCLI_ReadmeOnlyOnlyTouchesSkill(t *testing.T) {
	dir := stageMinimalCLIDir(t)
	rootPath := filepath.Join(dir, "internal/cli/root.go")
	rootBefore, _ := os.ReadFile(rootPath)

	if _, err := sweepCLI(dir, sweepOpts{ReadmeOnly: true}); err != nil {
		t.Fatalf("readme-only: %v", err)
	}

	rootAfter, _ := os.ReadFile(rootPath)
	if string(rootBefore) != string(rootAfter) {
		t.Error("readme-only sweep modified root.go; expected untouched")
	}
	skill, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if !strings.Contains(string(skill), "Automatic Learning") {
		t.Error("readme-only sweep did not patch SKILL.md")
	}
}

// TestSweep_FullRunOnFreshCLI_AllFilesEmittedAndAnchorBootstrapped
// is the integration test for the U13.5 anchor-bootstrap + stub-emit
// retrofit. A pre-anchor library CLI (no learn-migrations marker, no
// internal/cli/teach.go, no internal/cli/learn_init.go, no
// internal/learn/* package) is the input; the sweep must produce a
// directory that carries:
//
//   - The learn-migrations anchor + canonical block in store.go
//   - The bumped StoreSchemaVersion
//   - The internal/learn/* data package (every emitted template
//     present)
//   - internal/cli/teach.go + internal/cli/learn_init.go
//   - A patched root.go with the no-learn flag, AddCommand calls, and
//     skip-list
//   - A patched SKILL.md with the Automatic Learning section
//   - A bumped printing_press_version in .printing-press.json
//
// A second run on the same directory must report statusUnchanged with
// zero file mutations. This is the binding contract for U14: the
// pilot's first sweep is a real bootstrap, and a re-sweep produces
// zero textual diff.
func TestSweep_FullRunOnFreshCLI_AllFilesEmittedAndAnchorBootstrapped(t *testing.T) {
	dir := stagePreAnchorCLIDir(t)

	// First run: full bootstrap path.
	st1, err := sweepCLI(dir, sweepOpts{})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if st1 != statusPatched {
		t.Fatalf("first run expected patched; got %q", st1)
	}

	// Verify every artifact landed.
	expectations := map[string][]string{
		"internal/store/store.go": {
			learnMigrationAnchor,
			"search_learnings",
			"search_patterns",
			"entity_lookups",
			"teach_log_metadata",
			learnMigrationsBlockEndMarker,
			"const StoreSchemaVersion = 3",
		},
		"internal/cli/root.go": {
			`BoolVar(&flags.noLearn, "no-learn"`,
			"newTeachCmd(",
			"newRecallCmd(",
			"newLearningsCmd(",
			"learnHookSkipList",
		},
		"SKILL.md": {
			"Automatic Learning",
			"<!-- pp-learn-section-start -->",
		},
		".printing-press.json": {
			learnPressVersion,
		},
	}
	for rel, mustContain := range expectations {
		full := filepath.Join(dir, rel)
		body, err := os.ReadFile(full)
		if err != nil {
			t.Errorf("read %s: %v", rel, err)
			continue
		}
		for _, want := range mustContain {
			if !strings.Contains(string(body), want) {
				t.Errorf("%s missing %q after bootstrap sweep", rel, want)
			}
		}
	}

	// Newly emitted package files must all exist.
	expectedNewFiles := []string{
		"internal/cli/teach.go",
		"internal/cli/learn_init.go",
		"internal/learn/doc.go",
		"internal/learn/normalize.go",
		"internal/learn/match.go",
		"internal/learn/recall.go",
		"internal/learn/teach.go",
		"internal/learn/teach_log.go",
		"internal/learn/preseed.go",
		"internal/learn/entities/config.go",
		"internal/learn/entities/extract.go",
		"internal/learn/lookups/store.go",
		"internal/learn/lookups/seeds.go",
		"internal/learn/patterns/doc.go",
		"internal/learn/patterns/store.go",
		"internal/learn/patterns/extract.go",
		"internal/learn/patterns/apply.go",
	}
	for _, rel := range expectedNewFiles {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("expected emitted file %s missing: %v", rel, err)
		}
	}

	// Second run: zero diff. Snapshot every CLI-touched file's bytes
	// before the second sweep so we can confirm byte-for-byte
	// stability of the idempotent re-run.
	type fileSnap struct {
		rel   string
		bytes []byte
	}
	var snaps []fileSnap
	for _, rel := range append(expectedNewFiles,
		"internal/store/store.go",
		"internal/cli/root.go",
		"SKILL.md",
		".printing-press.json",
	) {
		body, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("snapshot read %s: %v", rel, err)
		}
		snaps = append(snaps, fileSnap{rel: rel, bytes: body})
	}

	st2, err := sweepCLI(dir, sweepOpts{})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if st2 != statusUnchanged {
		t.Errorf("second run expected unchanged; got %q", st2)
	}
	for _, snap := range snaps {
		got, err := os.ReadFile(filepath.Join(dir, snap.rel))
		if err != nil {
			t.Errorf("second-run read %s: %v", snap.rel, err)
			continue
		}
		if string(got) != string(snap.bytes) {
			t.Errorf("second run mutated %s; expected byte-for-byte stability", snap.rel)
		}
	}
}

// stageMinimalCLIDir creates the minimal file set the sweep needs to
// run successfully on a fixture CLI:
//
//   - .printing-press.json with required identity fields
//   - SKILL.md with an H1 so the learn section has an insertion point
//   - internal/cli/root.go in canonical shape
//   - internal/store/store.go with the learn-migrations anchor
//   - go.mod already declaring modernc.org/sqlite so `go mod tidy`
//     is skipped (the test doesn't shell out)
//
// Returns the directory path.
func stageMinimalCLIDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".printing-press.json"),
		`{"api_name":"demo","cli_name":"demo-pp-cli","printing_press_version":"0.0.0"}`)
	writeFile(t, filepath.Join(dir, "SKILL.md"),
		"---\nname: pp-demo\n---\n\n# Demo CLI\n\n## Usage\n\nstuff.\n")
	writeFile(t, filepath.Join(dir, "internal/cli/root.go"), canonicalRootFlagsShape)
	writeFile(t, filepath.Join(dir, "internal/store/store.go"), preLearnStoreSnippet)
	writeFile(t, filepath.Join(dir, "go.mod"),
		"module github.com/example/demo-pp-cli\n\ngo 1.26\n\nrequire (\n\tmodernc.org/sqlite v1.37.0\n)\n")
	return dir
}

// stagePreAnchorCLIDir mirrors stageMinimalCLIDir but uses the pre-U6
// store.go shape (no learn-migrations anchor). Exercises the
// bootstrap path: anchor + canonical block must be seeded in store.go
// during the first sweep, and a re-run must land in anchor mode with
// zero diff.
func stagePreAnchorCLIDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".printing-press.json"),
		`{"api_name":"demo","cli_name":"demo-pp-cli","printing_press_version":"0.0.0"}`)
	writeFile(t, filepath.Join(dir, "SKILL.md"),
		"---\nname: pp-demo\n---\n\n# Demo CLI\n\n## Usage\n\nstuff.\n")
	writeFile(t, filepath.Join(dir, "internal/cli/root.go"), canonicalRootFlagsShape)
	writeFile(t, filepath.Join(dir, "internal/store/store.go"), preLearnNoAnchorSnippet)
	writeFile(t, filepath.Join(dir, "go.mod"),
		"module github.com/example/demo-pp-cli\n\ngo 1.26\n\nrequire (\n\tmodernc.org/sqlite v1.37.0\n)\n")
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
