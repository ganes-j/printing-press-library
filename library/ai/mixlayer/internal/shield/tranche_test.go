// Copyright 2026 Cathryn Lavery and contributors. Licensed under Apache-2.0. See LICENSE.

package shield

import (
	"context"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/ai/mixlayer/internal/store"
)

func TestSplitRecordsCarriesCSVHeader(t *testing.T) {
	input := "name,email\nCathryn Lavery,cathryn@example.com\nJane Smith,jane@example.com\n"
	tranches := SplitRecords(input, 45)
	if len(tranches) < 2 {
		t.Fatalf("len(tranches) = %d, want at least 2", len(tranches))
	}
	for _, tr := range tranches {
		if !strings.HasPrefix(tr.Text, "name,email\n") {
			t.Fatalf("tranche missing CSV header: %q", tr.Text)
		}
	}
}

func TestRedactUsesSharedVaultAcrossTranches(t *testing.T) {
	s, err := store.Open(t.TempDir() + "/data.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	first, err := Redact(ctx, s, "Cathryn Lavery <cathryn@example.com>", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Redact(ctx, s, "Reminder for Cathryn Lavery", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first.Text, "PERSON_1") || !strings.Contains(second.Text, "PERSON_1") {
		t.Fatalf("shared vault did not keep PERSON_1 consistent: %q / %q", first.Text, second.Text)
	}
}
