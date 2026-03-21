package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/llm"
)

func newDiagnoseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diagnose <description>",
		Short: "Get AI-powered troubleshooting advice",
		Long:  "Searches historical troubleshooting notes and uses LLM to provide contextual advice. Falls back to FTS5 search without LLM.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]

			// Always search history first (works without LLM)
			logs, err := db.SearchTroubleshootLogs(query)
			if err != nil {
				logs = nil // degrade gracefully
			}

			if len(logs) > 0 {
				fmt.Println("Related troubleshooting history:")
				for _, l := range logs {
					fmt.Printf("  [%s] %s\n", l.Tags, l.Symptom)
					if l.Resolution != "" {
						fmt.Printf("    -> %s\n", l.Resolution)
					}
				}
				fmt.Println()
			}

			// If LLM available, get AI analysis
			if !llmRouter.Available() {
				if len(logs) == 0 {
					fmt.Println("No matching history found. Configure an LLM provider for AI-powered diagnosis:")
					fmt.Println("  nethelper config llm --help")
				}
				return nil
			}

			fmt.Println("")
			fmt.Println("AI Analysis:")

			// Build context from history
			var historyCtx string
			if len(logs) > 0 {
				var parts []string
				for _, l := range logs {
					parts = append(parts, fmt.Sprintf("- Symptom: %s / Finding: %s / Resolution: %s", l.Symptom, l.Findings, l.Resolution))
				}
				historyCtx = "Related past issues:\n" + strings.Join(parts, "\n")
			}

			systemPrompt := `You are a senior network engineer assistant. Analyze the problem description and provide:
1. Possible root causes (ranked by likelihood)
2. Recommended troubleshooting commands to run
3. If similar past issues are provided, reference them in your analysis
Reply in the same language as the user's input.`

			userMsg := fmt.Sprintf("Problem: %s", query)
			if historyCtx != "" {
				userMsg += "\n\n" + historyCtx
			}

			resp, err := llmRouter.Chat(context.Background(), llm.CapAnalyze, llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "system", Content: systemPrompt},
					{Role: "user", Content: userMsg},
				},
			})
			if err != nil {
				fmt.Printf("LLM error: %v\n", err)
				return nil // degrade gracefully
			}

			fmt.Println(resp.Content)
			return nil
		},
	}
}
