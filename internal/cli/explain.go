package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/llm"
)

func newExplainCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "explain [text-or-command]",
		Short: "AI-powered explanation of config or command output",
		Long:  "Explain network device configuration or command output. Reads from argument, --file, or stdin.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var text string
			if file != "" {
				data, err := os.ReadFile(file)
				if err != nil {
					return fmt.Errorf("read file: %w", err)
				}
				text = string(data)
			} else if len(args) > 0 {
				text = args[0]
			} else {
				return fmt.Errorf("provide text as argument or use --file")
			}

			if !llmRouter.Available() {
				fmt.Println("No LLM provider configured. Showing raw text:")
				fmt.Println(text)
				fmt.Println("\nConfigure an LLM for AI explanations: nethelper config llm --help")
				return nil
			}

			systemPrompt := `You are a senior network engineer. Explain the following network device configuration or command output.
Focus on:
1. What each section/line does
2. Key values and what they mean
3. Any potential issues or notable configurations
Reply in the same language as the user's input. Be concise and practical.`

			resp, err := llmRouter.Chat(context.Background(), llm.CapExplain, llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "system", Content: systemPrompt},
					{Role: "user", Content: text},
				},
			})
			if err != nil {
				return fmt.Errorf("LLM error: %w", err)
			}

			fmt.Println(resp.Content)
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "read from file instead of argument")
	return cmd
}
