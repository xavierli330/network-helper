package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/codegen"
	"github.com/xavierli/nethelper/internal/discovery"
	"github.com/xavierli/nethelper/internal/store"
	"github.com/xavierli/nethelper/internal/studio"
)

func newRuleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rule",
		Short: "Parser Rule Studio — manage adaptive parser rules",
	}
	cmd.AddCommand(newRuleStudioCmd())
	cmd.AddCommand(newRuleDiscoverCmd())
	cmd.AddCommand(newRuleListCmd())
	cmd.AddCommand(newRuleRegenCmd())
	cmd.AddCommand(newRuleHistoryCmd())
	cmd.AddCommand(newRuleFieldsCmd(fieldRegistry, registry))
	return cmd
}

func newRuleStudioCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "studio",
		Short: "Start the Rule Studio web UI",
		RunE: func(cmd *cobra.Command, args []string) error {
			if port == 0 {
				port = cfg.Rule.StudioPort
			}
			if port == 0 {
				port = 7070
			}
			eng := discovery.New(db, llmRouter)
			generateFn := studio.GenerateFn(func(rule store.PendingRule, testCases []store.RuleTestCase, repoRoot, approvedBy string) (string, error) {
				return codegen.Generate(rule, testCases, codegen.GeneratorOptions{
					RepoRoot:   repoRoot,
					ApprovedBy: approvedBy,
				})
			})
			srv := studio.NewServer(db, eng, llmRouter, generateFn, fieldRegistry)
			addr := fmt.Sprintf(":%d", port)
			fmt.Printf("🔬 Rule Studio running at http://localhost%s\nPress Ctrl+C to stop.\n", addr)
			return srv.ListenAndServe(addr)
		},
	}
	cmd.Flags().IntVar(&port, "port", 0, "HTTP port (default: config studio_port or 7070)")
	return cmd
}

func newRuleDiscoverCmd() *cobra.Command {
	var vendor string
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Run discovery engine to generate rule drafts",
		RunE: func(cmd *cobra.Command, args []string) error {
			eng := discovery.New(db, llmRouter)
			n, err := eng.RunOnce(cmd.Context(), vendor)
			if err != nil {
				return err
			}
			fmt.Printf("Discovery complete: %d new rule drafts created.\n", n)
			return nil
		},
	}
	cmd.Flags().StringVar(&vendor, "vendor", "", "Limit to specific vendor (default: all)")
	return cmd
}

func newRuleListCmd() *cobra.Command {
	var status string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pending rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			rules, err := db.ListPendingRules(status, 50)
			if err != nil {
				return err
			}
			if len(rules) == 0 {
				fmt.Println("No rules found.")
				return nil
			}
			fmt.Printf("%-6s %-10s %-45s %-14s %-10s\n", "ID", "Vendor", "Command Pattern", "Type", "Status")
			fmt.Println(strings.Repeat("-", 90))
			for _, r := range rules {
				fmt.Printf("%-6d %-10s %-45s %-14s %-10s\n",
					r.ID, r.Vendor, truncate(r.CommandPattern, 44), r.OutputType, r.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&status, "status", "", "Filter: draft|testing|approved|rejected")
	return cmd
}

func newRuleRegenCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "regen <rule-id>",
		Short: "Regenerate Go files for an approved rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid rule id: %s", args[0])
			}
			rule, err := db.GetPendingRule(id)
			if err != nil {
				return fmt.Errorf("rule %d not found: %w", id, err)
			}
			if rule.MergedAt != nil && !force {
				return fmt.Errorf("rule was merged at %s — use --force to regenerate",
					rule.MergedAt.Format(time.RFC3339))
			}
			if rule.MergedAt != nil && force {
				repoRoot, _ := os.Getwd()
				relPath := codegen.TargetFilePath(rule.Vendor, rule.CommandPattern)
				existing, _ := os.ReadFile(filepath.Join(repoRoot, relPath))
				generated, _ := codegen.GenerateParserFile(rule)
				if string(existing) != generated {
					fmt.Printf("WARNING: File %s differs from what will be generated.\nProceeding with --force...\n", relPath)
				}
			}
			testCases, err := db.ListRuleTestCases(id)
			if err != nil {
				return err
			}
			repoRoot, _ := os.Getwd()
			prURL, err := codegen.Generate(rule, testCases, codegen.GeneratorOptions{
				RepoRoot:   repoRoot,
				ApprovedBy: rule.ApprovedBy,
			})
			if err != nil {
				return err
			}
			fmt.Printf("PR created: %s\n", prURL)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Force regeneration even if already merged")
	return cmd
}

func newRuleHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history <vendor> <command>",
		Short: "Show history for a command pattern",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			rules, err := db.ListPendingRules("", 100)
			if err != nil {
				return err
			}
			found := false
			for _, summary := range rules {
				if summary.Vendor == args[0] && summary.CommandPattern == args[1] {
					r, err := db.GetPendingRule(summary.ID)
					if err != nil {
						continue
					}
					fmt.Printf("ID: %d  Status: %-10s  PR: %s  Created: %s\n",
						r.ID, r.Status, r.PRURL, r.CreatedAt.Format("2006-01-02 15:04"))
					found = true
				}
			}
			if !found {
				fmt.Println("No rule history found.")
			}
			return nil
		},
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
