// Copyright 2026 Cathryn Lavery and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/ai/mixlayer/internal/store"
	"github.com/spf13/cobra"
)

func TestPrepareShieldAskPayloadRedactsQuestionPII(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	prepared, err := prepareShieldAskPayload(
		ctx,
		s,
		"Alice Johnson bought from alice@example.com.",
		"What is the risk for John Smith with SSN 123-45-6789?",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"Alice Johnson", "alice@example.com", "John Smith", "123-45-6789"} {
		if strings.Contains(prepared.Payload, raw) {
			t.Fatalf("payload still contains raw PII %q: %s", raw, prepared.Payload)
		}
	}
	if len(prepared.Leaks) != 0 {
		t.Fatalf("payload leaks = %#v, want none", prepared.Leaks)
	}
	if prepared.MaskedEntities < 4 {
		t.Fatalf("masked entities = %d, want at least 4", prepared.MaskedEntities)
	}
}

func TestWriteShieldIngestResultPrintsCorpusWithoutOutput(t *testing.T) {
	var out bytes.Buffer
	cmd := newTestOutputCmd(&out)
	if err := writeShieldIngestResult(cmd, &rootFlags{}, "", "masked corpus", map[string]any{"tranches": 1, "output": ""}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "masked corpus" {
		t.Fatalf("stdout = %q, want masked corpus", out.String())
	}
}

func TestWriteShieldIngestResultIncludesCorpusInJSONMode(t *testing.T) {
	var out bytes.Buffer
	cmd := newTestOutputCmd(&out)
	if err := writeShieldIngestResult(cmd, &rootFlags{asJSON: true}, "", "masked corpus", map[string]any{"tranches": 1, "output": ""}); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["masked_corpus"] != "masked corpus" {
		t.Fatalf("masked_corpus = %#v, want masked corpus", got["masked_corpus"])
	}
}

func newTestOutputCmd(out *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	return cmd
}
