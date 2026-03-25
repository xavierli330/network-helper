package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/memory"
)

func newKnowledgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "knowledge",
		Short: "Direct knowledge base search without LLM",
		Long: `Search knowledge bases directly without LLM processing.

This command provides direct access to knowledge sources (IMA, local, HTTP)
with full traceability of results. Use this when you need:
- Verifiable data sources
- No LLM hallucination
- Raw search results from specific sources

Examples:
  nethelper knowledge search "骨干网排障" --source ima
  nethelper knowledge search "MPLS配置" --source local
  nethelper knowledge search "BGP故障" --all`,
	}
	cmd.AddCommand(newKnowledgeSearchCmd())
	cmd.AddCommand(newKnowledgeSourcesCmd())
	return cmd
}

func newKnowledgeSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search knowledge bases directly",
		Long: `Search knowledge bases directly without LLM interpretation.

The command searches configured knowledge sources and returns raw results
with full source attribution. No LLM is used to generate or augment responses.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceFilter, _ := cmd.Flags().GetString("source")
			searchAll, _ := cmd.Flags().GetBool("all")
			topK, _ := cmd.Flags().GetInt("limit")
			query := args[0]

			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			// Build knowledge sources from config
			var sources []memory.KnowledgeSource
			for _, sc := range cfg.Knowledge.Sources {
				if !sc.Enabled {
					continue
				}

				// Filter by source if specified
				if sourceFilter != "" && sc.Name != sourceFilter && sc.Type != sourceFilter {
					continue
				}

				switch sc.Type {
				case "http":
					if sc.URL == "" {
						continue
					}
					name := sc.Name
					if name == "" {
						name = sc.URL
					}
					sources = append(sources, memory.NewHTTPKnowledgeSource(name, sc.URL, sc.Token))
				case "ima":
					if sc.ClientID == "" || sc.APIKey == "" || sc.KBID == "" {
						continue
					}
					name := sc.Name
					if name == "" {
						name = "ima"
					}
					src := memory.NewIMAKnowledgeSource(sc.ClientID, sc.APIKey, sc.KBID, name)
					if src != nil {
						sources = append(sources, src)
					}
				}
			}

			// Add local knowledge source if data dir is configured and not filtered out
			if (searchAll || sourceFilter == "" || sourceFilter == "local") && cfg.DataDir != "" {
				embedder := llm.BuildEmbedder(cfg.Embedding)
				if embedder != nil {
					kb := memory.LoadKnowledge(context.Background(), cfg.DataDir+"/knowledge", embedder, db)
					localSrc := memory.NewLocalKnowledgeAdapter(kb, embedder)
					if localSrc != nil {
						sources = append(sources, localSrc)
					}
				}
			}

			if len(sources) == 0 {
				fmt.Println("No knowledge sources configured or matched.")
				fmt.Println("Configure sources in ~/.nethelper/config.yaml")
				return nil
			}

			// Search all sources
			agg := memory.NewAggregator(sources...)
			results := agg.SearchAll(context.Background(), query, topK)

			if len(results) == 0 {
				fmt.Println("No results found in any knowledge source.")
				fmt.Println()
				fmt.Println("Searched sources:")
				for _, src := range sources {
					fmt.Printf("  - %s\n", src.Name())
				}
				return nil
			}

			// Group results by source
			bySource := make(map[string][]memory.SearchResult)
			for _, r := range results {
				source := r.Source
				if idx := strings.Index(source, ":"); idx > 0 {
					source = source[:idx]
				}
				bySource[source] = append(bySource[source], r)
			}

			// Output results
			fmt.Printf("Query: \"%s\"\n", query)
			fmt.Printf("Found %d results from %d sources\n\n", len(results), len(bySource))

			for source, sourceResults := range bySource {
				fmt.Printf("═══ Source: %s (%d results) ═══\n\n", source, len(sourceResults))
				for i, r := range sourceResults {
					fmt.Printf("--- Result %d ---\n", i+1)
					fmt.Printf("Title:   %s\n", r.Title)
					fmt.Printf("Source:  %s\n", r.Source)
					fmt.Printf("Score:   %.2f\n", r.Score)
					fmt.Println()
					content := r.Content
					if len(content) > 1000 {
						content = content[:1000] + "\n... (truncated)"
					}
					fmt.Println(content)
					fmt.Println()
				}
			}

			return nil
		},
	}

	cmd.Flags().StringP("source", "s", "", "Filter by source name (ima, local, http, etc.)")
	cmd.Flags().BoolP("all", "a", false, "Search all sources including local")
	cmd.Flags().IntP("limit", "n", 10, "Maximum number of results")

	return cmd
}

func newKnowledgeSourcesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sources",
		Short: "List configured knowledge sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg == nil {
				return fmt.Errorf("config not loaded")
			}

			fmt.Println("Configured Knowledge Sources:")
			fmt.Println()

			if len(cfg.Knowledge.Sources) == 0 {
				fmt.Println("  No external knowledge sources configured.")
			} else {
				for _, sc := range cfg.Knowledge.Sources {
					status := "✓ enabled"
					if !sc.Enabled {
						status = "✗ disabled"
					}
					fmt.Printf("  • %s (%s) - %s\n", sc.Name, sc.Type, status)
					if sc.Type == "ima" {
						fmt.Printf("    KB ID: %s\n", sc.KBID)
					} else if sc.Type == "http" {
						fmt.Printf("    URL: %s\n", sc.URL)
					}
				}
			}

			if cfg.DataDir != "" {
				fmt.Println()
				fmt.Println("  • local (embedded) - ✓ enabled")
				fmt.Printf("    Path: %s/knowledge\n", cfg.DataDir)
			}

			fmt.Println()
			fmt.Println("Configure sources in ~/.nethelper/config.yaml")
			return nil
		},
	}
}
