// Copyright 2026 Cathryn Lavery and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/ai/mixlayer/internal/ladder"
	"github.com/mvanhorn/printing-press-library/library/ai/mixlayer/internal/pricing"
	"github.com/mvanhorn/printing-press-library/library/ai/mixlayer/internal/store"
	"github.com/spf13/cobra"
)

// pp:data-source live
func newLadderCmd(flags *rootFlags) *cobra.Command {
	var rungsSpec, dbPath string
	var reasoning bool
	var seed int64
	cmd := &cobra.Command{
		Use:         "ladder <question>",
		Short:       "Run one prompt across selected Mixlayer model rungs",
		Example:     `  mixlayer-pp-cli ladder "Which option is cheapest?" --reasoning --json`,
		Annotations: map[string]string{"mcp:hidden": "true", "pp:no-error-path-probe": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			var seedPtr *int64
			if seed != 0 {
				seedPtr = &seed
			}
			rungs := ladder.Rungs(rungsSpec)
			results := ladder.AskAcross(cmd.Context(), c, args[0], rungs, reasoning, seedPtr)
			s, err := openMixStore(cmd.Context(), dbPath)
			if err == nil {
				defer s.Close()
				groupID := store.NewID("ladder")
				_ = s.SaveLadder(cmd.Context(), groupID, args[0], rungs, ladder.FirstConfident(results), "")
				for _, res := range results {
					raw, _ := json.Marshal(res)
					_ = s.SaveRun(cmd.Context(), store.RunRecord{
						ID: store.NewID("run"), GroupID: groupID, Command: "ladder", Prompt: args[0],
						Answer: res.Answer, Reasoning: res.Reasoning, Model: res.Model, Seed: seed,
						RawJSON: raw, PromptTokens: res.PromptTokens, CompletionTokens: res.CompletionTokens,
						TotalTokens: res.TotalTokens, CostUSD: res.CostUSD, LatencyMS: res.LatencyMS,
					})
				}
			}
			return outputJSON(cmd, map[string]any{"results": results, "first_confident_model": ladder.FirstConfident(results)})
		},
	}
	cmd.Flags().StringVar(&rungsSpec, "rungs", "all", "Comma-separated model rungs or all")
	cmd.Flags().BoolVar(&reasoning, "reasoning", false, "Request reasoning_content and compare it across rungs")
	cmd.Flags().Int64Var(&seed, "seed", 0, "Best-effort deterministic seed")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path")
	return cmd
}

func newEscalateCmd(flags *rootFlags) *cobra.Command {
	var confidence float64
	var rungsSpec string
	cmd := &cobra.Command{
		Use:         "escalate <question>",
		Short:       "Climb the model ladder only until a rung is confident enough",
		Example:     `  mixlayer-pp-cli escalate "Classify these tickets" --confidence 0.85 --json`,
		Annotations: map[string]string{"mcp:hidden": "true", "pp:no-error-path-probe": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			var results []ladder.Result
			for _, rung := range ladder.Rungs(rungsSpec) {
				part := ladder.AskAcross(cmd.Context(), c, args[0]+"\n\nReturn a concise answer. If uncertain, say so.", []string{rung}, false, nil)
				results = append(results, part...)
				if len(part) > 0 && part[0].Error == "" && heuristicConfidence(part[0].Answer) >= confidence {
					break
				}
			}
			total := 0.0
			for _, r := range results {
				total += r.CostUSD
			}
			baseline := pricing.Estimate("qwen/qwen3.5-397b-a17b", 1000, 1000)
			return outputJSON(cmd, map[string]any{"results": results, "cost_usd": total, "frontier_baseline_usd": baseline, "saved_usd": baseline - total})
		},
	}
	cmd.Flags().Float64Var(&confidence, "confidence", 0.85, "Confidence threshold")
	cmd.Flags().StringVar(&rungsSpec, "rungs", "all", "Comma-separated model rungs or all")
	return cmd
}

func newCouncilCmd(flags *rootFlags) *cobra.Command {
	var membersSpec, judge string
	cmd := &cobra.Command{
		Use:         "council <question>",
		Short:       "Fan out to several rungs and ask a judge model to synthesize",
		Example:     `  mixlayer-pp-cli council "Pick the safest launch plan" --json`,
		Annotations: map[string]string{"mcp:hidden": "true", "pp:no-error-path-probe": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			results := ladder.AskAcross(cmd.Context(), c, args[0], ladder.Rungs(membersSpec), true, nil)
			var b strings.Builder
			fmt.Fprintf(&b, "Question: %s\n\nSynthesize these model answers and flag disagreements:\n", args[0])
			for _, r := range results {
				fmt.Fprintf(&b, "\nModel: %s\nReasoning: %s\nAnswer: %s\n", r.Model, r.Reasoning, r.Answer)
			}
			judgeResult := ladder.AskAcross(cmd.Context(), c, b.String(), []string{judge}, false, nil)
			return outputJSON(cmd, map[string]any{"members": results, "judge": judgeResult})
		},
	}
	cmd.Flags().StringVar(&membersSpec, "members", "qwen/qwen3.5-4b-free,qwen/qwen3.5-27b,qwen/qwen3.5-397b-a17b", "Comma-separated member models")
	cmd.Flags().StringVar(&judge, "judge", defaultFrontierModel, "Judge model")
	return cmd
}

func heuristicConfidence(answer string) float64 {
	lower := strings.ToLower(answer)
	if strings.Contains(lower, "not sure") || strings.Contains(lower, "uncertain") || strings.Contains(lower, "cannot") {
		return 0.4
	}
	if len(strings.TrimSpace(answer)) > 80 {
		return 0.9
	}
	return 0.7
}
