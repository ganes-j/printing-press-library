package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
)

func TestPrintJSONAppliesSelect(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{Use: "test"}
	cmd.SetOut(&buf)
	flags := &rootFlags{selectFields: "domain,registrar"}

	err := flags.printJSON(cmd, map[string]any{
		"domain":     "anthropic.com",
		"registrar":  "292",
		"expires_at": "2033-10-02T18:10:32Z",
	})
	if err != nil {
		t.Fatalf("printJSON returned error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	if _, ok := got["domain"]; !ok {
		t.Fatalf("selected field domain missing: %+v", got)
	}
	if _, ok := got["registrar"]; !ok {
		t.Fatalf("selected field registrar missing: %+v", got)
	}
	if _, ok := got["expires_at"]; ok {
		t.Fatalf("unselected field expires_at should be omitted: %+v", got)
	}
}

func TestDryRunOKPrintsPreview(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{Use: "funding"}
	root := &cobra.Command{Use: "company-goat-pp-cli"}
	root.AddCommand(cmd)
	cmd.SetOut(&buf)
	flags := &rootFlags{dryRun: true, asJSON: true}

	if !dryRunOK(cmd, flags) {
		t.Fatal("dryRunOK should return true when dry-run is set")
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("dry-run output is not JSON: %v\n%s", err, buf.String())
	}
	if got["dry_run"] != true {
		t.Fatalf("dry_run should be true: %+v", got)
	}
	if got["command"] != "company-goat-pp-cli funding" {
		t.Fatalf("unexpected command path: %+v", got)
	}
}
