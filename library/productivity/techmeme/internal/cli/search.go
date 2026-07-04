// Copyright 2026 Dave Morin and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	// searchAnchorRE matches href-first anchors in the results region. Techmeme
	// renders result content (publication cites and headlines) as uppercase
	// <A HREF="…">text</A>; the case-insensitive match keeps the parser working
	// if that casing shifts, since structural pairing filters non-story anchors.
	searchAnchorRE = regexp.MustCompile(`(?i)<A HREF="([^"]+)"[^>]*>([^<]+)</A>`)
	// searchIinfDateRE captures the idate text inside each result's iinf block
	// (e.g. "May 22, 2025, 11:35 AM").
	searchIinfDateRE = regexp.MustCompile(`(?is)<div class="iinf">\s*<span class="idate">([^<]*)</span>`)
	// searchPrevNextRE marks the pagination block; everything after it is
	// pagination links and sponsored /r2/ promos, never results.
	searchPrevNextRE = regexp.MustCompile(`(?i)<div class="prevnext">`)
	// searchCanvasRE / searchSponsorsRE bound the results region. Nav chrome
	// before the results canvas and sponsor promos after it are never results;
	// zero-hit pages carry neither an items div nor a prevnext block, so these
	// bounds are what keep header/footer anchors from counting as candidates.
	searchCanvasRE   = regexp.MustCompile(`(?i)class="resultscanvas"`)
	searchSponsorsRE = regexp.MustCompile(`(?i)class="sponsorscanvas"`)
)

// searchResult is one story record parsed from Techmeme's archive search
// results page (/search/d3results.jsp).
type searchResult struct {
	Num      int    `json:"num"`
	Source   string `json:"source"`
	Headline string `json:"headline"`
	Link     string `json:"link"`
	Date     string `json:"date"`
}

func newSearchCmd(flags *rootFlags) *cobra.Command {
	var days int

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search Techmeme headlines",
		Long: `Search Techmeme for headlines matching a query.
Supports quoted phrases, wildcards, +/-, AND/OR/NOT, sourcename:X.

Results come from Techmeme's live archive search (back to ~2005) and each
record carries a date field (ISO YYYY-MM-DD, empty when unparseable).
Use --days N to keep only results from the last N days.`,
		Example: `  # Search for a topic
  techmeme-pp-cli search "artificial intelligence"

  # Search with source filter
  techmeme-pp-cli search "AI sourcename:Bloomberg"

  # Search as JSON
  techmeme-pp-cli search "Apple" --json

  # Only results from the last 30 days
  techmeme-pp-cli search "Apple" --days 30 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}

			query := strings.Join(args, " ")

			c, err := flags.newClient()
			if err != nil {
				return err
			}

			params := map[string]string{
				"q":  query,
				"wm": "false",
			}

			data, err := c.Get("/search/d3results.jsp", params)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			results, anchorsWithoutDates := parseSearchResults(string(data))
			if days > 0 {
				results = filterSearchByDays(results, days, time.Now())
			}
			return renderSearchResults(cmd, flags, query, results, anchorsWithoutDates)
		},
	}

	cmd.Flags().IntVar(&days, "days", 0, "Only include results dated within the last N days (0 = no filter; undated results are dropped when set)")

	return cmd
}

// parseSearchResults extracts story records from a Techmeme search results
// page by walking anchors and iinf date blocks in document order. Each result
// on the page is a publication anchor, then a headline anchor, then a
// <div class="iinf"> date block — so the last candidate anchor before each
// iinf block is that story's headline, and anchors without a trailing date
// block (nav links, publication cites) are consumed without being emitted.
// Everything after the prevnext pagination div is ignored.
//
// The second return value reports that candidate anchors were found but no
// iinf date blocks — a signal that Techmeme's markup shifted and the parse is
// likely silently incomplete.
func parseSearchResults(page string) ([]searchResult, bool) {
	// Scope the scan to the results canvas when the markers are present:
	// header/nav chrome before it, sponsor promos after it, and everything
	// after the prevnext pagination div are never results.
	if loc := searchCanvasRE.FindStringIndex(page); loc != nil {
		page = page[loc[1]:]
	}
	if loc := searchSponsorsRE.FindStringIndex(page); loc != nil {
		page = page[:loc[0]]
	}
	if loc := searchPrevNextRE.FindStringIndex(page); loc != nil {
		page = page[:loc[0]]
	}

	type anchorTok struct {
		pos   int
		href  string
		title string
	}
	var anchors []anchorTok
	for _, m := range searchAnchorRE.FindAllStringSubmatchIndex(page, -1) {
		href := page[m[2]:m[3]]
		title := cleanSearchTitle(page[m[4]:m[5]])

		if strings.Contains(href, "/r2/") {
			continue
		}
		if strings.Contains(href, "techmeme.com/") && strings.Contains(title, "context") {
			continue
		}
		anchors = append(anchors, anchorTok{pos: m[0], href: href, title: title})
	}

	dates := searchIinfDateRE.FindAllStringSubmatchIndex(page, -1)
	if len(dates) == 0 {
		return []searchResult{}, len(anchors) > 0
	}

	results := []searchResult{}
	ai := 0
	for _, d := range dates {
		// The story headline is the last candidate anchor before this iinf
		// block; earlier anchors in the same span are publication/nav links.
		var story *anchorTok
		for ai < len(anchors) && anchors[ai].pos < d[0] {
			story = &anchors[ai]
			ai++
		}
		if story == nil {
			continue
		}
		results = append(results, searchResult{
			Num:      len(results) + 1,
			Source:   extractDomain(story.href),
			Headline: story.title,
			Link:     story.href,
			Date:     parseSearchDate(page[d[2]:d[3]]),
		})
	}

	return results, false
}

// cleanSearchTitle strips tags and decodes HTML entities in anchor text
// (&nbsp; &mdash; &amp; &ldquo; …).
func cleanSearchTitle(s string) string {
	s = stripHTML(s)
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return strings.TrimSpace(html.UnescapeString(s))
}

// parseSearchDate converts an iinf date like "May 22, 2025, 11:35 AM" or
// "Oct 18, 2022" to ISO YYYY-MM-DD. Returns "" when the text is unparseable,
// mirroring parseRSSDate/parseRiverTimestamp's multi-layout loop.
func parseSearchDate(s string) string {
	s = strings.TrimSpace(s)
	layouts := []string{
		"January 2, 2006, 3:04 PM",
		"Jan 2, 2006, 3:04 PM",
		"January 2, 2006",
		"Jan 2, 2006",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

// filterSearchByDays keeps results dated within the last N days relative to
// now. Undated records are dropped when the filter is active because their
// recency cannot be verified. days <= 0 disables filtering.
func filterSearchByDays(results []searchResult, days int, now time.Time) []searchResult {
	if days <= 0 {
		return results
	}
	cutoff := now.AddDate(0, 0, -days)
	out := make([]searchResult, 0, len(results))
	for _, r := range results {
		if r.Date == "" {
			continue
		}
		t, err := time.Parse("2006-01-02", r.Date)
		if err != nil || t.Before(cutoff) {
			continue
		}
		r.Num = len(out) + 1
		out = append(out, r)
	}
	return out
}

// renderSearchResults writes search output. JSON mode always emits a valid
// JSON array on stdout — an empty array for zero hits, never prose — so piped
// agent consumers can rely on parseable output. Human mode keeps the friendly
// zero-results line and a table otherwise.
func renderSearchResults(cmd *cobra.Command, flags *rootFlags, query string, results []searchResult, warnNoDates bool) error {
	if warnNoDates {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: search results page contained candidate anchors but no iinf date blocks; Techmeme markup may have changed and results may be incomplete")
	}

	if flags.asJSON {
		if results == nil {
			// json.Marshal renders a nil slice as null, not [].
			results = []searchResult{}
		}
		return printJSONFiltered(cmd.OutOrStdout(), results, flags)
	}

	if len(results) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No results for %q\n", query)
		return nil
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "%d results for %q\n", len(results), query)

	headers := []string{"#", "DATE", "SOURCE", "HEADLINE"}
	rows := make([][]string, 0, len(results))
	for _, r := range results {
		rows = append(rows, []string{
			fmt.Sprintf("%d", r.Num),
			r.Date,
			truncate(r.Source, 25),
			truncate(r.Headline, 70),
		})
	}
	return flags.printTable(cmd, headers, rows)
}

func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	host := u.Hostname()
	host = strings.TrimPrefix(host, "www.")
	return host
}
