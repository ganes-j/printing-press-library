package main

import (
	"strings"
	"testing"
)

// canonicalRootFlagsShape mirrors the rootFlags-struct shape every
// newer printed CLI ships. The sweep operates on this shape.
const canonicalRootFlagsShape = `package cli

import (
	"context"
	"github.com/spf13/cobra"
)

type rootFlags struct {
	OutputJSON bool
	Verbose    bool
}

func Execute() error {
	var flags rootFlags
	rootCmd := &cobra.Command{
		Use: "demo-pp-cli",
	}
	rootCmd.PersistentFlags().BoolVar(&flags.OutputJSON, "json", false, "json output")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		_ = cmd
		_ = context.TODO()
		return nil
	}
	rootCmd.AddCommand(newResourceCmd(&flags))
	rootCmd.AddCommand(newSyncCmd(&flags))
	return rootCmd.Execute()
}
`

// legacyRootShape mirrors the agent-capture / instacart shape:
// package-global rootCmd with no rootFlags struct. The sweep refuses
// to patch this.
const legacyRootShape = `package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use: "agent-capture",
}

func Execute() error {
	return rootCmd.Execute()
}
`

func TestDetectRootShape(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want rootShape
	}{
		{"canonical-rootFlags-struct", canonicalRootFlagsShape, rootShapeFlagsStruct},
		{"legacy-var-rootCmd", legacyRootShape, rootShapeLegacy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := detectRootShape([]byte(tc.src))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got shape %d, want %d", got, tc.want)
			}
		})
	}
}

func TestPatchRootAST_InjectsAllPieces(t *testing.T) {
	ctx := sweepCtx{CLIName: "demo-pp-cli", APIName: "demo"}
	got, changed, err := patchRootAST(canonicalRootFlagsShape, ctx)
	if err != nil {
		t.Fatalf("patchRootAST: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true on first run")
	}
	expectations := []string{
		"noLearn bool",
		`BoolVar(&flags.noLearn, "no-learn"`,
		"learnCfg := newLearnConfig()",
		"rootCmd.AddCommand(newTeachCmd(&flags, learnCfg))",
		"rootCmd.AddCommand(newRecallCmd(&flags, learnCfg))",
		"rootCmd.AddCommand(newLearningsCmd(&flags, learnCfg))",
		"rootCmd.AddCommand(newTeachPatternCmd(&flags))",
		"rootCmd.AddCommand(newTeachLookupCmd(&flags))",
		"learnHookSkipList",
		"func shouldSkipLearnHook(",
	}
	for _, want := range expectations {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in patched root.go; got:\n%s", want, got)
		}
	}
}

func TestPatchRootAST_Idempotent(t *testing.T) {
	ctx := sweepCtx{CLIName: "demo-pp-cli", APIName: "demo"}
	first, _, err := patchRootAST(canonicalRootFlagsShape, ctx)
	if err != nil {
		t.Fatalf("first patch: %v", err)
	}
	second, changed, err := patchRootAST(first, ctx)
	if err != nil {
		t.Fatalf("second patch: %v", err)
	}
	if changed {
		t.Error("expected changed=false on second run")
	}
	if second != first {
		t.Errorf("second run produced diff:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestPatchRootAST_RefusesLegacyShape(t *testing.T) {
	// Shape detection runs upstream in sweepCLI; patchRootAST itself
	// is exercised here against the canonical shape only. This test
	// exists so a future contributor accidentally relaxing the shape
	// gate notices: the legacy fixture must report
	// rootShapeLegacy from detectRootShape.
	shape, err := detectRootShape([]byte(legacyRootShape))
	if err != nil {
		t.Fatalf("detectRootShape: %v", err)
	}
	if shape != rootShapeLegacy {
		t.Errorf("expected legacy shape detection; got %d", shape)
	}
}
