package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	config := &cobra.Command{
		Use:   "config",
		Short: "Configuration management",
	}
	config.AddCommand(newConfigLLMCmd())
	return config
}

func newConfigLLMCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "llm",
		Short: "Show LLM provider configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("LLM Configuration:")
			fmt.Printf("  Config file: %s\n\n", cfgFile)

			if cfg.LLM.Default != "" {
				fmt.Printf("  Default provider: %s\n", cfg.LLM.Default)
			} else {
				fmt.Println("  Default provider: (none)")
			}

			if len(cfg.LLM.Providers) > 0 {
				fmt.Println("\n  Providers:")
				for name, pc := range cfg.LLM.Providers {
					model := pc.Model
					if model == "" {
						model = "(default)"
					}
					baseURL := pc.BaseURL
					if baseURL == "" {
						baseURL = "https://api.openai.com/v1"
					}
					hasKey := "no"
					if pc.APIKey != "" {
						hasKey = "yes"
					}
					fmt.Printf("    %s: model=%s base_url=%s api_key=%s\n", name, model, baseURL, hasKey)
				}
			} else {
				fmt.Println("\n  No providers configured.")
			}

			if len(cfg.LLM.Routing) > 0 {
				fmt.Println("\n  Capability routing:")
				for cap, provider := range cfg.LLM.Routing {
					fmt.Printf("    %s -> %s\n", cap, provider)
				}
			}

			if !llmRouter.Available() {
				fmt.Println("\n  Warning: No LLM provider is active. To configure, edit:")
				fmt.Printf("    %s\n\n", cfgFile)
				fmt.Println("  Example config:")
				fmt.Println("    llm:")
				fmt.Println("      default: ollama")
				fmt.Println("      providers:")
				fmt.Println("        ollama:")
				fmt.Println("          base_url: http://localhost:11434")
				fmt.Println("          model: qwen2.5:14b")
			} else {
				fmt.Println("\n  LLM provider is active.")
			}

			return nil
		},
	}
}
