package cli

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/mvanhorn/printing-press-library/library/food-and-dining/recipe-goat/internal/recipes"

	"github.com/spf13/cobra"
)

// trendingEntry is one homepage link we surface as "what's on the front page today".
type trendingEntry struct {
	Site  string `json:"site"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

func newTrendingCmd(flags *rootFlags) *cobra.Command {
	var (
		siteFilter string
		limit      int
	)
	cmd := &cobra.Command{
		Use:         "trending",
		Short:       "Show the top recipes currently featured on each site's homepage",
		Example:     "  recipe-goat-pp-cli trending --site budgetbytes,recipetineats --limit 5",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			sites := siteHostsFromCSV(siteFilter)
			// Dry-run skips the homepage fan-out: useful for CI / structural
			// verifiers and to advertise which sites would be queried without
			// hitting the network.
			if flags.dryRun {
				if flags.asJSON {
					return flags.printJSON(cmd, []trendingEntry{})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "trending: dry-run; would query %d site(s) (limit=%d)\n", len(sites), limit)
				return nil
			}
			client := httpClientForSites(flags.timeout)
			ctx, cancel := flags.withContext()
			defer cancel()

			var mu sync.Mutex
			all := []trendingEntry{}
			siteErrors := map[string]string{} // hostname -> short reason
			emptySites := []string{}          // reached but returned no recipes
			var wg sync.WaitGroup
			sem := make(chan struct{}, 4)
			for _, s := range sites {
				s := s
				wg.Add(1)
				go func() {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					body, err := recipes.FetchHTML(ctx, client, "https://www."+s.Hostname+"/")
					if err != nil {
						mu.Lock()
						siteErrors[s.Hostname] = shortErr(err)
						mu.Unlock()
						return
					}
					// Candidates are permissive (up to limit*4). Validate each
					// by fetching its JSON-LD; keep only real Recipe pages.
					res := validateTrendingCandidates(ctx, client, body, s, limit)
					mu.Lock()
					if len(res) == 0 {
						emptySites = append(emptySites, s.Hostname)
					}
					all = append(all, res...)
					mu.Unlock()
				}()
			}
			wg.Wait()

			// Surface per-site failures on stderr so partial runs are obvious.
			// JSON output keeps stderr silent-free? No — warnings go to stderr
			// either way so scripts can still capture JSON on stdout cleanly.
			for host, why := range siteErrors {
				fmt.Fprintf(cmd.ErrOrStderr(), "warn: %s unreachable: %s\n", host, why)
			}
			for _, host := range emptySites {
				fmt.Fprintf(cmd.ErrOrStderr(), "warn: %s reachable but no recipes found on homepage\n", host)
			}

			if flags.asJSON {
				return flags.printJSON(cmd, all)
			}
			if len(all) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no trending results — sites may be blocking)")
				return nil
			}
			headers := []string{"SITE", "TITLE", "URL"}
			rows := make([][]string, 0, len(all))
			for _, e := range all {
				rows = append(rows, []string{e.Site, truncate(e.Title, 60), e.URL})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().StringVar(&siteFilter, "site", "all", "Sites to query (CSV or 'all')")
	cmd.Flags().IntVar(&limit, "limit", 5, "Max per site")
	return cmd
}

// shortErr condenses an error to a single-line reason tag for stderr warnings.
// Full error chains are too noisy when we're already listing 37 sites.
func shortErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	const max = 80
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// validateTrendingCandidates pulls candidate links from homepage HTML (using
// the same URL-pattern filter as search), then fetches each candidate and
// requires a valid Recipe JSON-LD node before accepting it. This guarantees
// trending never returns category pages, random round-ups, or the site's
// "welcome" post — only actual recipes that exist at those URLs right now.
func validateTrendingCandidates(ctx context.Context, client *http.Client, body []byte, site recipes.Site, limit int) []trendingEntry {
	// Pull up to limit*4 candidate anchors — we'll filter aggressively.
	candidates := recipes.ExtractSearchResults(body, site, limit*4)
	out := make([]trendingEntry, 0, limit)
	for _, c := range candidates {
		if strings.Contains(strings.ToLower(c.Title), "skip to") {
			continue
		}
		// Validate by fetching the full JSON-LD. Budget one per candidate,
		// stop once we have `limit` confirmed recipes.
		r, err := recipes.Fetch(ctx, client, c.URL)
		if err != nil || r == nil || r.Name == "" || len(r.RecipeIngredient) == 0 {
			continue
		}
		title := r.Name
		if title == "" {
			title = c.Title
		}
		out = append(out, trendingEntry{Site: site.Hostname, Title: title, URL: c.URL})
		if len(out) >= limit {
			break
		}
	}
	return out
}
