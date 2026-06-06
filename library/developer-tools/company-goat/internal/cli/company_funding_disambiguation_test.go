package cli

import (
	"testing"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/company-goat/internal/source/sec"
)

func TestSummarizeByCIKGroupsAndSorts(t *testing.T) {
	filings := []sec.FormD{
		{CIK: "0001890586", EntityName: "Notion Capital Mgmt", State: "DE", FilingDate: "2019-04-12"},
		{CIK: "0001890586", EntityName: "Notion Capital Mgmt", State: "DE", FilingDate: "2022-08-01"},
		{CIK: "0001999111", EntityName: "Notion Labs Inc.", State: "DE", FilingDate: "2024-09-15"},
	}
	got := summarizeByCIK(filings)
	if len(got) != 2 {
		t.Fatalf("expected 2 CIK groups, got %d", len(got))
	}
	// Sorted by latest filing date descending — Notion Labs (2024) before
	// Notion Capital (2022). This is the ordering an agent looking for
	// "the most-recently-active entity matching this name" would pick first.
	if got[0].CIK != "0001999111" {
		t.Fatalf("expected most-recent CIK first; got %s", got[0].CIK)
	}
	if got[0].FilingCount != 1 {
		t.Errorf("notion labs filing count: want 1, got %d", got[0].FilingCount)
	}
	if got[1].FilingCount != 2 {
		t.Errorf("notion capital filing count: want 2, got %d", got[1].FilingCount)
	}
	if got[0].LatestFilingDate != "2024-09-15" {
		t.Errorf("notion labs latest date: want 2024-09-15, got %s", got[0].LatestFilingDate)
	}
}

func TestSummarizeByCIKEmptyInput(t *testing.T) {
	if got := summarizeByCIK(nil); len(got) != 0 {
		t.Errorf("nil input should produce empty summary, got %d", len(got))
	}
	if got := summarizeByCIK([]sec.FormD{}); len(got) != 0 {
		t.Errorf("empty input should produce empty summary, got %d", len(got))
	}
}

func TestFilterFilingsByCIKToleratesLeadingZeros(t *testing.T) {
	filings := []sec.FormD{
		{CIK: "0001890586", EntityName: "A"},
		{CIK: "0001999111", EntityName: "B"},
		{CIK: "0001999111", EntityName: "B"},
	}
	// Caller drops leading zeros (common when copy-pasting from EDGAR's
	// human-readable display).
	got := filterFilingsByCIK(filings, "1999111")
	if len(got) != 2 {
		t.Fatalf("expected 2 filings for CIK 1999111, got %d", len(got))
	}
	for _, fd := range got {
		if fd.CIK != "0001999111" {
			t.Errorf("filter leaked unrelated CIK: %s", fd.CIK)
		}
	}
}

func TestFilterFilingsByCIKReturnsEmptyForNoMatch(t *testing.T) {
	filings := []sec.FormD{{CIK: "0001890586", EntityName: "A"}}
	got := filterFilingsByCIK(filings, "9999999")
	if len(got) != 0 {
		t.Errorf("no-match filter should return empty, got %d", len(got))
	}
}

func TestBuildFundingResultMarksAmbiguousWhenMultipleCIKs(t *testing.T) {
	filings := []sec.FormD{
		{CIK: "0001", EntityName: "A", FilingDate: "2024-01-01"},
		{CIK: "0002", EntityName: "B", FilingDate: "2024-01-02"},
	}
	r := buildFundingResult("example.com", filings, nil)
	if !r.IsAmbiguous {
		t.Errorf("expected IsAmbiguous=true for multi-CIK result")
	}
	if len(r.CIKSummaries) != 2 {
		t.Errorf("expected 2 CIK summaries, got %d", len(r.CIKSummaries))
	}
}

func TestBuildFundingResultNotAmbiguousForSingleCIK(t *testing.T) {
	filings := []sec.FormD{
		{CIK: "0001", EntityName: "A", FilingDate: "2024-01-01"},
		{CIK: "0001", EntityName: "A", FilingDate: "2023-06-01"},
	}
	r := buildFundingResult("example.com", filings, nil)
	if r.IsAmbiguous {
		t.Errorf("expected IsAmbiguous=false for single-CIK result")
	}
	if len(r.CIKSummaries) != 1 {
		t.Errorf("expected 1 CIK summary, got %d", len(r.CIKSummaries))
	}
}
