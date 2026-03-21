package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/llm"
)

func newDiagnoseCmd() *cobra.Command {
	var deviceID string
	cmd := &cobra.Command{
		Use:   "diagnose <description>",
		Short: "AI-powered troubleshooting advice using device data from nethelper",
		Long: `Describe a problem and get AI analysis based on actual device data.
The tool gathers device config, neighbors, interfaces, and troubleshooting
history from the database, then asks the LLM for diagnosis.

Falls back to FTS5 keyword search when no LLM is configured.

Examples:
  nethelper diagnose "OSPF 邻居 down"
  nethelper diagnose --device core-01 "BGP 邻居震荡"
  nethelper diagnose "为什么去往 172.16.0.0/16 的流量走了次优路径"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]

			// 1. Search troubleshooting history (always, even without LLM)
			logs, err := db.SearchTroubleshootLogs(query)
			if err != nil {
				logs = nil
			}

			if len(logs) > 0 {
				fmt.Println("📋 相关排障历史:")
				for _, l := range logs {
					fmt.Printf("  [%s] %s\n", l.Tags, l.Symptom)
					if l.Findings != "" {
						fmt.Printf("    发现: %s\n", l.Findings)
					}
					if l.Resolution != "" {
						fmt.Printf("    解决: %s\n", l.Resolution)
					}
				}
				fmt.Println()
			}

			// 2. If no LLM, stop here
			if !llmRouter.Available() {
				if len(logs) == 0 {
					fmt.Println("没有匹配的历史记录。配置 LLM 可获得 AI 排障建议:")
					fmt.Println("  nethelper config llm --help")
				}
				return nil
			}

			// 3. Gather device context from nethelper memory
			deviceCtx := gatherContext(query, deviceID)

			// 4. Build history context
			var historyCtx string
			if len(logs) > 0 {
				var parts []string
				for _, l := range logs {
					parts = append(parts, fmt.Sprintf("- 现象: %s / 发现: %s / 解决: %s", l.Symptom, l.Findings, l.Resolution))
				}
				historyCtx = "## 相关排障历史\n" + strings.Join(parts, "\n")
			}

			// 5. Call LLM with full context
			fmt.Println("")
			fmt.Println("🤖 AI 分析:")

			systemPrompt := `You are a senior network engineer assistant. You have access to a network management database.
The user describes a network problem. Below is the relevant device data and troubleshooting history from the database.

Your job:
1. Analyze the problem using the provided device data (config, neighbors, interfaces, routes)
2. Identify possible root causes (ranked by likelihood), referencing specific data from the context
3. Suggest troubleshooting commands to run next
4. If similar past issues exist, reference them
5. If the data is insufficient, say what additional information is needed

Be precise. Reference specific interface names, IP addresses, AS numbers from the context.
Reply in the same language as the user's input.`

			userMsg := fmt.Sprintf("问题描述: %s", query)
			if deviceCtx != "" {
				userMsg += "\n\n" + deviceCtx
			}
			if historyCtx != "" {
				userMsg += "\n\n" + historyCtx
			}

			resp, err := llmRouter.Chat(context.Background(), llm.CapAnalyze, llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "system", Content: systemPrompt},
					{Role: "user", Content: userMsg},
				},
				MaxTokens: 4096,
			})
			if err != nil {
				fmt.Printf("LLM error: %v\n", err)
				return nil
			}

			fmt.Println(resp.Content)
			return nil
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "focus diagnosis on a specific device")
	return cmd
}
