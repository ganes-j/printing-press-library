package cli

import (
	"testing"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/company-goat/internal/source/sec"
)

func TestBinMentionJuneLifeCanonical(t *testing.T) {
	cases := []struct {
		name string
		hit  sec.SearchHit
		want string
	}{
		{
			name: "Weber 10-K with EX-21 subsidiary list",
			hit: sec.SearchHit{
				Form:         "10-K",
				DisplayNames: []string{"Weber Inc.  (CIK 0001890586)"},
				FileDate:     "2021-12-14",
			},
			want: "subsidiary",
		},
		{
			name: "Venture Lending & Leasing VII portfolio mention",
			hit: sec.SearchHit{
				Form:         "10-Q",
				DisplayNames: []string{"Venture Lending & Leasing VII, Inc.  (CIK 0001503520)"},
				FileDate:     "2019-08-14",
			},
			want: "debt",
		},
		{
			name: "Venture Lending & Leasing VIII portfolio mention",
			hit: sec.SearchHit{
				Form:         "10-K",
				DisplayNames: []string{"Venture Lending & Leasing VIII, Inc."},
				FileDate:     "2020-03-16",
			},
			want: "debt",
		},
		{
			name: "Weber 8-K with subsidiary mention",
			hit: sec.SearchHit{
				Form:         "8-K",
				DisplayNames: []string{"Weber Inc."},
				FileDate:     "2021-11-18",
			},
			want: "acquisition",
		},
		{
			name: "Weber 10-K/A amendment is also a subsidiary signal",
			hit: sec.SearchHit{
				Form:         "10-K/A",
				DisplayNames: []string{"Weber Inc."},
				FileDate:     "2022-12-14",
			},
			want: "subsidiary",
		},
		{
			name: "Self-filing falls into Other",
			hit: sec.SearchHit{
				Form:         "10-K",
				DisplayNames: []string{"June Life Inc.  (CIK 0001234567)"},
				FileDate:     "2022-01-01",
			},
			want: "other",
		},
		{
			name: "S-1 from non-self filer falls into Other",
			hit: sec.SearchHit{
				Form:         "S-1",
				DisplayNames: []string{"Crucible Acquisition Corp"},
				FileDate:     "2020-12-18",
			},
			want: "other",
		},
		{
			name: "DEF 14A from non-self filer falls into Other",
			hit: sec.SearchHit{
				Form:         "DEF 14A",
				DisplayNames: []string{"Lucira Health, Inc."},
				FileDate:     "2022-04-20",
			},
			want: "other",
		},
		{
			name: "Empty display names skips self-filing detection but still classifies by form",
			hit: sec.SearchHit{
				Form:         "10-K",
				DisplayNames: []string{},
				FileDate:     "2022-01-01",
			},
			want: "subsidiary",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := binMention(tc.hit, "june life")
			if got != tc.want {
				t.Errorf("binMention(%+v) = %q, want %q", tc.hit, got, tc.want)
			}
		})
	}
}

func TestBinMentionDoesNotFalsePositiveOnSubstringFiler(t *testing.T) {
	// "Junebug Inc." contains "june" as a substring but is not the same
	// company as "June Life". Per Risks section of the plan, the binning
	// uses the bigram phrase "june life", not the bare stem, so this is
	// not classified as self-filing.
	hit := sec.SearchHit{
		Form:         "10-K",
		DisplayNames: []string{"Junebug Inc."},
	}
	if got := binMention(hit, "june life"); got != "subsidiary" {
		t.Errorf("expected subsidiary classification on Junebug 10-K (not self-filing), got %q", got)
	}
}

func TestAccessionURLShape(t *testing.T) {
	hit := sec.SearchHit{
		CIKs:      []string{"0001890586"},
		Accession: "0001628280-21-024546",
	}
	want := "https://www.sec.gov/Archives/edgar/data/1890586/000162828021024546/"
	got := accessionURL(hit)
	if got != want {
		t.Errorf("accessionURL = %q, want %q", got, want)
	}
}

func TestAccessionURLEmptyHit(t *testing.T) {
	if got := accessionURL(sec.SearchHit{}); got != "" {
		t.Errorf("accessionURL on empty hit = %q, want empty", got)
	}
	if got := accessionURL(sec.SearchHit{CIKs: []string{"0001890586"}}); got != "" {
		t.Errorf("accessionURL with no accession = %q, want empty", got)
	}
}

func TestFirstDisplayNameStripsCIKAnnotation(t *testing.T) {
	cases := []struct {
		input []string
		want  string
	}{
		{[]string{"Weber Inc.  (CIK 0001890586)"}, "Weber Inc."},
		{[]string{"Venture Lending & Leasing VII, Inc.  (CIK 0001503520)"}, "Venture Lending & Leasing VII, Inc."},
		{[]string{"Stripe Milton LLC"}, "Stripe Milton LLC"},
		{[]string{}, ""},
		{[]string{""}, ""},
	}
	for _, tc := range cases {
		got := firstDisplayName(tc.input)
		if got != tc.want {
			t.Errorf("firstDisplayName(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
