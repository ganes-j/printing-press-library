package cli

import (
	"fmt"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/food-and-dining/recipe-goat/internal/recipes"

	"github.com/spf13/cobra"
)

func newSearchCmd(flags *rootFlags) *cobra.Command {
	var (
		siteFilter  string
		kidFriendly bool
		inSeason    bool
		limit       int
	)
	cmd := &cobra.Command{
		Use:         "search <query>",
		Short:       "Search recipe titles across curated sites without fetching the full recipes (returns metadata only)",
		Example:     "  recipe-goat-pp-cli search \"vegan curry\" --kid-friendly --limit 10",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			ctx, cancel := flags.withContext()
			defer cancel()
			client := httpClientForSites(flags.timeout)
			results, err := recipes.SearchAll(ctx, client, query, limit)
			if err != nil {
				return err
			}
			// Filter by site.
			if siteFilter != "" {
				filtered := []recipes.SearchResult{}
				for _, r := range results {
					if strings.Contains(r.Site, siteFilter) {
						filtered = append(filtered, r)
					}
				}
				results = filtered
			}
			// Kid-friendly: filter titles containing excluded ingredients.
			if kidFriendly {
				st, err := openRecipeStore()
				if err == nil {
					defer st.Close()
					excl, _ := st.KidExcluded()
					filtered := []recipes.SearchResult{}
					for _, r := range results {
						l := strings.ToLower(r.Title)
						ok := true
						for _, e := range excl {
							if strings.Contains(l, e) {
								ok = false
								break
							}
						}
						if ok {
							filtered = append(filtered, r)
						}
					}
					results = filtered
				}
			}
			if inSeason {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: --in-season marker not yet wired; showing all results")
			}
			if limit > 0 && len(results) > limit {
				results = results[:limit]
			}
			if flags.asJSON {
				return flags.printJSON(cmd, results)
			}
			if len(results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no results — sites may be blocking; run `doctor` to check reachability)")
				return nil
			}
			headers := []string{"SITE", "TITLE", "URL"}
			rows := make([][]string, 0, len(results))
			for _, r := range results {
				rows = append(rows, []string{r.Site, truncate(r.Title, 60), r.URL})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().StringVar(&siteFilter, "site", "", "Only include results from this site (substring match)")
	cmd.Flags().BoolVar(&kidFriendly, "kid-friendly", false, "Filter out results whose titles contain excluded ingredients")
	cmd.Flags().BoolVar(&inSeason, "in-season", false, "Flag out-of-season ingredients (wip)")
	cmd.Flags().IntVar(&limit, "limit", 30, "Max results")
	return cmd
}
