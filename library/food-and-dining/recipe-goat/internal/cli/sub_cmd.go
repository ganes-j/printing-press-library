package cli

import (
	"fmt"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/food-and-dining/recipe-goat/internal/recipes"

	"github.com/spf13/cobra"
)

func newSubCmd(flags *rootFlags) *cobra.Command {
	var (
		context string
		vegan   bool
		limit   int
	)
	cmd := &cobra.Command{
		Use:         "sub <ingredient>",
		Short:       "Look up culinary substitutions for an ingredient, optionally filtered by recipe context (baking, dairy-free, etc.)",
		Example:     "  recipe-goat-pp-cli sub buttermilk --context baking",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ingredient := strings.Join(args, " ")
			subs := recipes.LookupSubs(ingredient, context)
			if vegan {
				filtered := make([]recipes.Sub, 0, len(subs))
				for _, s := range subs {
					lower := strings.ToLower(s.Substitute)
					if strings.Contains(lower, "butter") || strings.Contains(lower, "milk") ||
						strings.Contains(lower, "yogurt") || strings.Contains(lower, "cream") ||
						strings.Contains(lower, "egg") || strings.Contains(lower, "cheese") {
						// Dairy/egg-containing substitute; skip when vegan.
						if !strings.Contains(lower, "oat milk") && !strings.Contains(lower, "coconut") && !strings.Contains(lower, "flax") && !strings.Contains(lower, "applesauce") {
							continue
						}
					}
					filtered = append(filtered, s)
				}
				subs = filtered
			}
			if limit > 0 && len(subs) > limit {
				subs = subs[:limit]
			}
			if flags.asJSON {
				return flags.printJSON(cmd, subs)
			}
			if len(subs) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "(no substitutions found for %q)\n", ingredient)
				return nil
			}
			headers := []string{"SUBSTITUTE", "RATIO", "CONTEXT", "SOURCE", "TRUST"}
			rows := make([][]string, 0, len(subs))
			for _, s := range subs {
				rows = append(rows, []string{
					s.Substitute,
					s.Ratio,
					s.Context,
					s.Source,
					fmt.Sprintf("%.2f", s.Trust),
				})
			}
			return flags.printTable(cmd, headers, rows)
		},
	}
	cmd.Flags().StringVar(&context, "context", "any", "Use context: baking|marinade|sauce|any")
	cmd.Flags().BoolVar(&vegan, "vegan", false, "Only suggest vegan substitutes")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max substitutions to show (0 = all)")
	return cmd
}
