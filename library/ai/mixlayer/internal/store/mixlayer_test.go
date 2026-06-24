// Copyright 2026 Cathryn Lavery and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestRunLedgerSearchAndVault(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	run := RunRecord{
		ID: "run_test", Command: "ask", Prompt: "Find revenue leakage",
		Answer: "Check churn cohorts", Reasoning: "Revenue changed after discounting",
		Model: "qwen/qwen3.5-4b-free", RawJSON: json.RawMessage(`{"ok":true}`),
	}
	if err := s.SaveRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	found, err := s.SearchRuns(ctx, "revenue", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != "run_test" {
		t.Fatalf("SearchRuns returned %#v", found)
	}
	if err := s.SaveVaultEntry(ctx, VaultEntry{Token: "EMAIL_1", Value: "cathryn@example.com", Kind: "EMAIL"}); err != nil {
		t.Fatal(err)
	}
	entry, ok, err := s.TokenForValue(ctx, "EMAIL", "cathryn@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || entry.Token != "EMAIL_1" {
		t.Fatalf("TokenForValue = %#v, %v", entry, ok)
	}
}
