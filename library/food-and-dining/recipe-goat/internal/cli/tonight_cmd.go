package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/food-and-dining/recipe-goat/internal/store"

	"github.com/spf13/cobra"
)

func newTonightCmd(flags *rootFlags) *cobra.Command {
	var (
		maxTime     time.Duration
		noRepeat    string
		kidFriendly bool
		vegetarian  bool
		tag         string
		limit       int
	)
	cmd := &cobra.Command{
		Use:         "tonight",
		Short:       "Pick dinner from your saved cookbook by max cook time, recency, and dietary tags",
		Example:     "  recipe-goat-pp-cli tonight --max-time 30m --no-repeat-within 7d --kid-friendly",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openRecipeStore()
			if err != nil {
				return err
			}
			defer st.Close()

			recs, err := st.ListRecipes(tag, "", "", 10000, 0)
			if err != nil {
				return err
			}

			// Time filter.
			if maxTime > 0 {
				max := int(maxTime.Seconds())
				filtered := []*store.StoredRecipe{}
				for _, r := range recs {
					if r.TotalTimeS > 0 && r.TotalTimeS <= max {
						filtered = append(filtered, r)
					}
				}
				recs = filtered
			}

			// Kid-friendly filter.
			if kidFriendly {
				excl, err := st.KidExcluded()
				if err == nil && len(excl) > 0 {
					filtered := []*store.StoredRecipe{}
					for _, r := range recs {
						joined := strings.ToLower(strings.Join(r.Ingredients, " ") + " " + r.Title)
						bad := false
						for _, e := range excl {
							if strings.Contains(joined, e) {
								bad = true
								break
							}
						}
						if !bad {
							filtered = append(filtered, r)
						}
					}
					recs = filtered
				}
			}

			// Vegetarian: exclude common meat words.
			if vegetarian {
				meats := []string{"chicken", "beef", "pork", "bacon", "sausage", "turkey", "lamb", "ham", "anchovy", "salmon", "tuna", "shrimp", "fish"}
				filtered := []*store.StoredRecipe{}
				for _, r := range recs {
					joined := strings.ToLower(strings.Join(r.Ingredients, " ") + " " + r.Title)
					bad := false
					for _, m := range meats {
						if strings.Contains(joined, m) {
							bad = true
							break
						}
					}
					if !bad {
						filtered = append(filtered, r)
					}
				}
				recs = filtered
			}

			// No-repeat-within.
			if noRepeat != "" {
				d, err := parseDurationShorthand(noRepeat)
				if err != nil {
					return usageErr(fmt.Errorf("invalid --no-repeat-within: %w", err))
				}
				ids := make([]int64, 0, len(recs))
				for _, r := range recs {
					ids = append(ids, r.ID)
				}
				lastMap, _ := st.LastCookedMap(ids)
				cutoff := time.Now().Add(-d)
				filtered := []*store.StoredRecipe{}
				for _, r := range recs {
					if last, ok := lastMap[r.ID]; ok {
						if last.After(cutoff) {
							continue
						}
					}
					filtered = append(filtered, r)
				}
				recs = filtered
			}

			// Rank by freshness (days since last cook) × rating.
			ids := make([]int64, 0, len(recs))
			for _, r := range recs {
				ids = append(ids, r.ID)
			}
			lastMap, _ := st.LastCookedMap(ids)
			now := time.Now()
			type scored struct {
				r     *store.StoredRecipe
				score float64
			}
			scoredList := make([]scored, 0, len(recs))
			for _, r := range recs {
				daysSince := 365.0
				if last, ok := lastMap[r.ID]; ok {
					daysSince = now.Sub(last).Hours() / 24
				}
				freshness := daysSince
				if freshness > 365 {
					freshness = 365
				}
				freshness /= 365 // 0..1
				rating := r.Rating / 5.0
				if rating <= 0 {
					rating = 0.5
				}
				scoredList = append(scoredList, scored{r: r, score: 0.6*freshness + 0.4*rating})
			}
			sort.SliceStable(scoredList, func(i, j int) bool { return scoredList[i].score > scoredList[j].score })
			if limit > 0 && len(scoredList) > limit {
				scoredList = scoredList[:limit]
			}

			if flags.asJSON {
				out := make([]map[string]any, 0, len(scoredList))
				for _, s := range scoredList {
					out = append(out, map[string]any{
						"id":         s.r.ID,
						"title":      s.r.Title,
						"site":       s.r.Site,
						"totalTimeS": s.r.TotalTimeS,
						"rating":     s.r.Rating,
						"score":      s.score,
					})
				}
				return flags.printJSON(cmd, out)
			}
			if len(scoredList) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(no recipes match — try relaxing --max-time or --no-repeat-within)")
				return nil
			}
			headers := []string{"ID", "TITLE", "TIME", "RATING", "SCORE"}
			rows := make([][]string, 0, len(scoredList))
			for _, s := range scoredList {
				rating := "—"
				if s.r.Rating > 0 {
					rating = fmt.Sprintf("%.2f", s.r.Rating)
				}
				rows = append(rows, []string{
					strconv.FormatInt(s.r.ID, 10),
					truncate(s.r.Title, 60),
					formatDuration(s.r.TotalTimeS),
					rating,
					fmt.Sprintf("%.3f", s.score),
				})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().DurationVar(&maxTime, "max-time", 0, "Max total time (e.g., 30m)")
	cmd.Flags().StringVar(&noRepeat, "no-repeat-within", "", "Don't suggest recipes cooked within this window (e.g., 7d)")
	cmd.Flags().BoolVar(&kidFriendly, "kid-friendly", false, "Filter against the kid-exclusion list")
	cmd.Flags().BoolVar(&vegetarian, "vegetarian", false, "Exclude common meats")
	cmd.Flags().StringVar(&tag, "tag", "", "Filter by cookbook tag")
	cmd.Flags().IntVar(&limit, "limit", 3, "Max suggestions")
	return cmd
}
