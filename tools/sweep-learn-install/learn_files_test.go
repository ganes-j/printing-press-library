package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// goldenLearnAPIDir is the path (relative to a developer checkout) to
// the cli-printing-press golden fixture for the generate-learn-loop-api
// case. The parity test resolves it by walking up from the sweep tool
// directory. When the checkout isn't a sibling, the test SKIPs rather
// than fails — the parity test is a developer-machine signal, not a CI
// blocker.
const goldenLearnAPIDir = "testdata/golden/expected/generate-learn-loop-api/learn-loop-example"

// candidateCLIPrintingPressPaths lists locations the test will probe
// looking for a usable cli-printing-press checkout. The first one
// returning a readable golden fixture wins.
func candidateCLIPrintingPressPaths() []string {
	home := os.Getenv("HOME")
	return []string{
		filepath.Join(home, "cli-printing-press"),
		filepath.Join(home, ".claude", "worktrees", "cli-printing-press"),
		// Common sibling layouts.
		"../../../cli-printing-press",
		"../../../../cli-printing-press",
	}
}

// TestRenderLearnPackage_ByteForByteParity asserts that the sweep tool
// emits every learn-package file byte-for-byte identical to what the
// generator's golden fixture carries. This is the contract that lets
// the sweep retrofit existing CLIs without drifting from fresh-print
// output.
//
// The golden fixture used:
//
//	cli-printing-press/testdata/golden/expected/
//	    generate-learn-loop-api/learn-loop-example/internal/learn/...
//
// Owner / Name / modulePath are extracted from the fixture's own
// header comment so the parity check stays accurate when those values
// change upstream.
func TestRenderLearnPackage_ByteForByteParity(t *testing.T) {
	goldenRoot := findGoldenLearnFixture(t)
	if goldenRoot == "" {
		t.Skip("cli-printing-press golden fixture not found; parity test is developer-only")
	}

	ctx := sweepCtx{
		// Mirror the generator's spec values for the
		// learn-loop-example fixture (printing-press-golden owner;
		// learn-loop-example api/name; learn-loop-example-pp-cli
		// module).
		CLIDir:     "/tmp/parity-target",
		CLIName:    "learn-loop-example-pp-cli",
		APIName:    "learn-loop-example",
		Category:   "other",
		OwnerName:  "printing-press-golden",
		ModulePath: "learn-loop-example-pp-cli",
	}
	emitted, err := renderLearnPackage(ctx)
	if err != nil {
		t.Fatalf("renderLearnPackage: %v", err)
	}

	// Only assert parity for files the golden fixture carries. The
	// learn-loop-example artifacts.txt only locks one file per
	// subpackage (recall.go, extract.go, seeds.go, apply.go) plus
	// store.go in internal/store/.
	parityFiles := []string{
		"internal/learn/recall.go",
		"internal/learn/entities/extract.go",
		"internal/learn/lookups/seeds.go",
		"internal/learn/patterns/apply.go",
	}
	for _, rel := range parityFiles {
		t.Run(rel, func(t *testing.T) {
			goldenPath := filepath.Join(goldenRoot, rel)
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Skipf("golden file not present: %v", err)
			}
			got, ok := emitted[rel]
			if !ok {
				t.Fatalf("sweep did not emit %s; templates may not be embedded", rel)
			}
			// Normalize the copyright year so the test does not break
			// at midnight UTC every January 1st. The generator stamps
			// the fixture once, and the sweep stamps the current year
			// — both are correct by their own contract. The structural
			// parity (imports, identifiers, body) is what we want to
			// lock here.
			wantNormalized := stripCopyrightYear(string(want))
			gotNormalized := stripCopyrightYear(string(got))
			if wantNormalized != gotNormalized {
				t.Errorf("byte-for-byte parity mismatch for %s\n--- want ---\n%s\n--- got ---\n%s",
					rel, wantNormalized, gotNormalized)
			}
		})
	}
}

// TestEmitsTeachGo_ByteForByteParity asserts the sweep's emission of
// internal/cli/teach.go matches the cli-printing-press generator's
// own emission byte-for-byte. teach.go is the cobra surface that
// wires newTeachCmd / newRecallCmd / newLearningsCmd / newTeachPatternCmd
// / newTeachLookupCmd into root.go; the file does not depend on any
// spec.Learn shape (no TickerPatterns / Stopwords / EntityLookupSeeds
// gates), so the golden's learn-loop-example output and the sweep's
// stub emission are textually equivalent. Year is normalized as
// elsewhere.
func TestEmitsTeachGo_ByteForByteParity(t *testing.T) {
	goldenRoot := findGoldenLearnFixture(t)
	if goldenRoot == "" {
		t.Skip("cli-printing-press golden fixture not found; parity test is developer-only")
	}

	ctx := sweepCtx{
		CLIDir:     "/tmp/parity-target",
		CLIName:    "learn-loop-example-pp-cli",
		APIName:    "learn-loop-example",
		Category:   "other",
		OwnerName:  "printing-press-golden",
		ModulePath: "learn-loop-example-pp-cli",
	}
	emitted, err := renderLearnPackage(ctx)
	if err != nil {
		t.Fatalf("renderLearnPackage: %v", err)
	}

	goldenPath := filepath.Join(goldenRoot, "internal", "cli", "teach.go")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Skipf("teach.go golden not present: %v", err)
	}
	got, ok := emitted["internal/cli/teach.go"]
	if !ok {
		t.Fatal("sweep did not emit internal/cli/teach.go")
	}
	wantNormalized := stripCopyrightYear(string(want))
	gotNormalized := stripCopyrightYear(string(got))
	if wantNormalized != gotNormalized {
		t.Errorf("byte-for-byte parity mismatch for internal/cli/teach.go\n--- want ---\n%s\n--- got ---\n%s",
			wantNormalized, gotNormalized)
	}
}

// TestEmitsLearnInitGo_StubDefaults asserts the sweep's emission of
// internal/cli/learn_init.go matches the canonical "empty Learn block"
// shape: a no-op newLearnConfig (returns entities.NewConfig() with no
// RegisterTickerPattern / RegisterStopwords calls) and a no-op
// initLearn (returns nil, never touches the DB). The cli-printing-press
// golden fixture for learn-loop-example has populated seeds, so the
// parity target here is an embedded expected string rather than a
// golden file — documented in the embed comment below.
func TestEmitsLearnInitGo_StubDefaults(t *testing.T) {
	ctx := sweepCtx{
		CLIDir:     "/tmp/stub-target",
		CLIName:    "demo-pp-cli",
		APIName:    "demo",
		Category:   "other",
		OwnerName:  "Tester",
		ModulePath: "github.com/example/demo-pp-cli",
	}
	emitted, err := renderLearnPackage(ctx)
	if err != nil {
		t.Fatalf("renderLearnPackage: %v", err)
	}
	got, ok := emitted["internal/cli/learn_init.go"]
	if !ok {
		t.Fatal("sweep did not emit internal/cli/learn_init.go")
	}

	// Structural assertions: the stub shape avoids the imports and
	// call sites that only render under a populated Learn block.
	gotStr := string(got)
	mustContain := []string{
		"package cli",
		"func newLearnConfig() *entities.Config {",
		"cfg := entities.NewConfig()",
		"return cfg",
		"func initLearn(ctx context.Context, db *sql.DB) error {",
		"_ = ctx",
		"_ = db",
		"return nil",
		"var learnInitOnce sync.Once",
		"func runLearnInitOnce(ctx context.Context) {",
		"github.com/example/demo-pp-cli/internal/learn/entities",
		"github.com/example/demo-pp-cli/internal/store",
	}
	for _, want := range mustContain {
		if !strings.Contains(gotStr, want) {
			t.Errorf("stub learn_init.go missing %q\nrendered:\n%s", want, gotStr)
		}
	}

	mustNotContain := []string{
		// regexp import + RegisterTickerPattern only render when
		// Learn.TickerPatterns is non-empty.
		`"regexp"`,
		"RegisterTickerPattern",
		// RegisterStopwords only renders when Learn.Stopwords is
		// non-empty.
		"RegisterStopwords",
		// lookups import + SeedFromConfig only render when
		// Learn.EntityLookupSeeds is non-empty.
		"learn/lookups",
		"SeedFromConfig",
	}
	for _, unwanted := range mustNotContain {
		if strings.Contains(gotStr, unwanted) {
			t.Errorf("stub learn_init.go unexpectedly contains %q (likely a Learn-shape gate fired):\nrendered:\n%s", unwanted, gotStr)
		}
	}
}

// TestStubLearnInitGoIsBenignNoOp asserts the stub learn_init.go
// parses as syntactically valid Go. The "benign no-op" claim in the
// task statement turns on the package compiling against the rest of
// the printed CLI; this test guards the upstream parse step (the
// sweep produces something gofmt+parser will accept) without
// requiring a full module compile in the test harness.
func TestStubLearnInitGoIsBenignNoOp(t *testing.T) {
	ctx := sweepCtx{
		CLIDir:     "/tmp/stub-target",
		CLIName:    "demo-pp-cli",
		APIName:    "demo",
		Category:   "other",
		OwnerName:  "Tester",
		ModulePath: "github.com/example/demo-pp-cli",
	}
	emitted, err := renderLearnPackage(ctx)
	if err != nil {
		t.Fatalf("renderLearnPackage: %v", err)
	}
	src, ok := emitted["internal/cli/learn_init.go"]
	if !ok {
		t.Fatal("sweep did not emit internal/cli/learn_init.go")
	}

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "learn_init.go", src, parser.ParseComments); err != nil {
		t.Fatalf("stub learn_init.go does not parse: %v\n--- source ---\n%s", err, src)
	}
}

// stripCopyrightYear normalizes the year token in the
// `// Copyright YYYY ...` header line so a year tick doesn't break
// parity. Only the YYYY digit run gets replaced; the rest of the
// header is preserved.
func stripCopyrightYear(s string) string {
	const prefix = "// Copyright "
	idx := strings.Index(s, prefix)
	if idx != 0 {
		return s
	}
	rest := s[len(prefix):]
	// Walk past digits.
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	return prefix + "YYYY" + rest[end:]
}

// findGoldenLearnFixture probes a small number of likely locations for
// a cli-printing-press checkout's golden learn fixture. Returns the
// path to the learn-loop-example directory if one is found, or ""
// otherwise.
func findGoldenLearnFixture(t *testing.T) string {
	t.Helper()
	for _, base := range candidateCLIPrintingPressPaths() {
		candidate := filepath.Join(base, goldenLearnAPIDir)
		if _, err := os.Stat(filepath.Join(candidate, "internal", "learn", "recall.go")); err == nil {
			return candidate
		}
	}
	return ""
}

// TestRenderLearnPackage_AllFilesPresent verifies the sweep emits the
// complete file set the generator emits for a learn-enabled spec:
// the internal/learn data package plus the internal/cli teach.go +
// learn_init.go cobra surface. A new template added to the generator
// but missed in the sweep would surface as a smaller emission count.
func TestRenderLearnPackage_AllFilesPresent(t *testing.T) {
	ctx := sweepCtx{
		CLIName:    "test-pp-cli",
		APIName:    "test",
		Category:   "other",
		OwnerName:  "Tester",
		ModulePath: "github.com/example/test-pp-cli",
	}
	emitted, err := renderLearnPackage(ctx)
	if err != nil {
		t.Fatalf("renderLearnPackage: %v", err)
	}
	expectedFiles := []string{
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
		// Cobra-surface files emitted alongside the learn data package.
		"internal/cli/teach.go",
		"internal/cli/learn_init.go",
	}
	for _, f := range expectedFiles {
		if _, ok := emitted[f]; !ok {
			t.Errorf("expected emitted file %s missing from sweep output", f)
		}
	}
	if testing.Verbose() {
		t.Logf("emitted %d files on %s/%s", len(emitted), runtime.GOOS, runtime.GOARCH)
	}
}

// TestRenderLearnPackage_Idempotent runs the renderer twice and
// asserts identical output. The renderer reads embedded templates and
// runs gofmt, both of which are pure; this test guards against a
// future change introducing non-determinism (e.g., a map iteration
// landing in a template).
func TestRenderLearnPackage_Idempotent(t *testing.T) {
	ctx := sweepCtx{
		CLIName:    "idem-pp-cli",
		APIName:    "idem",
		Category:   "other",
		OwnerName:  "Tester",
		ModulePath: "github.com/example/idem-pp-cli",
	}
	first, err := renderLearnPackage(ctx)
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	second, err := renderLearnPackage(ctx)
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("file count differs between runs: %d vs %d", len(first), len(second))
	}
	for rel, content := range first {
		other, ok := second[rel]
		if !ok {
			t.Errorf("file %s emitted in first run but not second", rel)
			continue
		}
		if string(content) != string(other) {
			t.Errorf("file %s differs between runs", rel)
		}
	}
}
