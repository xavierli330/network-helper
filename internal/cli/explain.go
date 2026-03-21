package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/llm"
)

func newExplainCmd() *cobra.Command {
	var file, deviceID string
	cmd := &cobra.Command{
		Use:   "explain [question-or-text]",
		Short: "AI-powered analysis using device data from nethelper memory",
		Long: `Ask questions about your network devices using data stored in nethelper.
The tool gathers relevant context (config, neighbors, routes, scratch pad)
from the database and sends it to the LLM along with your question.

Examples:
  nethelper explain "gz-hxy-xxx 有哪些 bgp 邻居"
  nethelper explain --device core-01 "这台设备的 OSPF 配置有什么问题"
  nethelper explain --file output.txt "解释这段输出"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var userQuestion string
			if file != "" {
				data, err := os.ReadFile(file)
				if err != nil {
					return fmt.Errorf("read file: %w", err)
				}
				userQuestion = string(data)
			} else if len(args) > 0 {
				userQuestion = strings.Join(args, " ")
			} else {
				return fmt.Errorf("provide a question as argument or use --file")
			}

			if !llmRouter.Available() {
				fmt.Println("No LLM provider configured.")
				fmt.Println("Configure an LLM: nethelper config llm --help")
				return nil
			}

			// Gather context from nethelper memory
			ctx := gatherContext(userQuestion, deviceID)

			systemPrompt := `You are a senior network engineer assistant with access to a network management database.
Below is the relevant data from the database for the user's question. Use this data to answer accurately.
If the data doesn't contain enough information, say what's missing and suggest which commands to run.
Reply in the same language as the user's question. Be precise and reference specific data from the context.`

			userMsg := userQuestion
			if ctx != "" {
				userMsg = userQuestion + "\n\n--- nethelper 数据库上下文 ---\n" + ctx
			}

			resp, err := llmRouter.Chat(context.Background(), llm.CapExplain, llm.ChatRequest{
				Messages: []llm.Message{
					{Role: "system", Content: systemPrompt},
					{Role: "user", Content: userMsg},
				},
				MaxTokens: 4096,
			})
			if err != nil {
				return fmt.Errorf("LLM error: %w", err)
			}

			fmt.Println(resp.Content)
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "read question/text from file")
	cmd.Flags().StringVar(&deviceID, "device", "", "focus on a specific device (improves context relevance)")
	return cmd
}

// gatherContext collects relevant data from the nethelper database.
// It tries to identify which device(s) the question is about, then pulls
// config, neighbors, interfaces, scratch pad entries as LLM context.
func gatherContext(question, explicitDevice string) string {
	var sections []string

	// Determine target device(s)
	targetDevices := findRelevantDevices(question, explicitDevice)

	if len(targetDevices) == 0 {
		// No specific device found — provide general overview
		devices, err := db.ListDevices()
		if err != nil || len(devices) == 0 {
			return ""
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("## 已知设备 (%d台)", len(devices)))
		for _, d := range devices {
			lines = append(lines, fmt.Sprintf("- %s (hostname=%s, vendor=%s, router_id=%s)", d.ID, d.Hostname, d.Vendor, d.RouterID))
		}
		return strings.Join(lines, "\n")
	}

	for _, devID := range targetDevices {
		dev, err := db.GetDevice(devID)
		if err != nil {
			continue
		}

		sections = append(sections, fmt.Sprintf("## 设备: %s (hostname=%s, vendor=%s, router_id=%s, mgmt_ip=%s)",
			dev.ID, dev.Hostname, dev.Vendor, dev.RouterID, dev.MgmtIP))

		// Interfaces
		ifaces, _ := db.GetInterfaces(devID)
		if len(ifaces) > 0 {
			var ifLines []string
			ifLines = append(ifLines, "### 接口列表")
			for _, i := range ifaces {
				ip := i.IPAddress
				if ip != "" && i.Mask != "" {
					ip = ip + "/" + i.Mask
				}
				ifLines = append(ifLines, fmt.Sprintf("- %s type=%s status=%s ip=%s desc=%s", i.Name, i.Type, i.Status, ip, i.Description))
			}
			sections = append(sections, strings.Join(ifLines, "\n"))
		}

		// Neighbors (from latest snapshot)
		snapID, err := db.LatestSnapshotID(devID)
		if err == nil {
			neighbors, _ := db.GetNeighbors(devID, snapID)
			if len(neighbors) > 0 {
				var nLines []string
				nLines = append(nLines, "### 协议邻居")
				for _, n := range neighbors {
					nLines = append(nLines, fmt.Sprintf("- protocol=%s remote_id=%s state=%s interface=%s area=%s as=%d uptime=%s",
						n.Protocol, n.RemoteID, n.State, n.LocalInterface, n.AreaID, n.ASNumber, n.Uptime))
				}
				sections = append(sections, strings.Join(nLines, "\n"))
			}
		}

		// Config snapshot (truncated to keep context manageable)
		configs, _ := db.GetConfigSnapshots(devID)
		if len(configs) > 0 {
			configText := configs[0].ConfigText
			// If config is huge, extract relevant sections based on question keywords
			relevantConfig := extractRelevantConfig(configText, question)
			if relevantConfig != "" {
				sections = append(sections, "### 相关配置片段\n```\n"+relevantConfig+"\n```")
			} else if len(configText) > 3000 {
				sections = append(sections, "### 配置摘要（前3000字符）\n```\n"+configText[:3000]+"\n...(truncated)\n```")
			} else {
				sections = append(sections, "### 完整配置\n```\n"+configText+"\n```")
			}
		}

		// Scratch pad entries for this device
		scratches, _ := db.ListScratch(devID, "", 5)
		if len(scratches) > 0 {
			var sLines []string
			sLines = append(sLines, "### 暂存区（最近的命令输出）")
			for _, s := range scratches {
				content := s.Content
				if len(content) > 2000 {
					content = content[:2000] + "\n...(truncated)"
				}
				sLines = append(sLines, fmt.Sprintf("#### %s [%s]\n```\n%s\n```", s.Query, s.Category, content))
			}
			sections = append(sections, strings.Join(sLines, "\n"))
		}
	}

	return strings.Join(sections, "\n\n")
}

// findRelevantDevices identifies which device(s) the question is about.
func findRelevantDevices(question, explicitDevice string) []string {
	if explicitDevice != "" {
		return []string{strings.ToLower(explicitDevice)}
	}

	// Try to match device IDs or hostnames mentioned in the question
	devices, err := db.ListDevices()
	if err != nil {
		return nil
	}

	lower := strings.ToLower(question)
	var matches []string
	for _, d := range devices {
		// Match by device ID, hostname, or partial hostname
		if strings.Contains(lower, strings.ToLower(d.ID)) ||
			strings.Contains(lower, strings.ToLower(d.Hostname)) {
			matches = append(matches, d.ID)
		}
	}

	// If no device matched but there's only one device in DB, use it
	if len(matches) == 0 && len(devices) == 1 {
		matches = []string{devices[0].ID}
	}

	return matches
}

// extractRelevantConfig extracts config sections relevant to the question.
// For example, if question mentions "bgp", extract the bgp section from config.
func extractRelevantConfig(fullConfig, question string) string {
	lower := strings.ToLower(question)

	// Keywords to config section headers
	keywords := map[string][]string{
		"bgp":     {"bgp"},
		"ospf":    {"ospf"},
		"isis":    {"isis"},
		"mpls":    {"mpls"},
		"acl":     {"acl"},
		"nat":     {"nat"},
		"vlan":    {"vlan"},
		"trunk":   {"eth-trunk", "link-aggregation"},
		"route":   {"route-policy", "ip route-static"},
		"policy":  {"route-policy", "traffic-policy"},
		"ntp":     {"ntp"},
		"snmp":    {"snmp"},
		"stp":     {"stp"},
		"vrf":     {"vpn-instance"},
		"vpn":     {"vpn-instance"},
		"tunnel":  {"tunnel"},
		"te":      {"mpls te", "tunnel"},
		"sr":      {"segment-routing"},
		"ldp":     {"mpls ldp"},
		"rsvp":    {"mpls rsvp"},
		"qos":     {"qos", "traffic-policy"},
		"接口":    {"interface"},
		"邻居":    {"bgp", "ospf", "isis"},
		"路由":    {"route-policy", "bgp", "ospf"},
		"策略":    {"route-policy", "traffic-policy", "acl"},
	}

	// Find which keywords match the question
	var sectionNames []string
	for keyword, sections := range keywords {
		if strings.Contains(lower, keyword) {
			sectionNames = append(sectionNames, sections...)
		}
	}

	if len(sectionNames) == 0 {
		return ""
	}

	// Extract matching sections from config (between # delimiters for Huawei)
	lines := strings.Split(fullConfig, "\n")
	var result []string
	inRelevantSection := false
	sectionDepth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Huawei config uses # as section delimiter
		if trimmed == "#" {
			if inRelevantSection && sectionDepth == 0 {
				result = append(result, line)
				inRelevantSection = false
				continue
			}
			inRelevantSection = false
			sectionDepth = 0
		}

		// Check if this line starts a relevant section
		if !inRelevantSection {
			lineLower := strings.ToLower(trimmed)
			for _, name := range sectionNames {
				if strings.Contains(lineLower, name) {
					inRelevantSection = true
					// Include the # before this section
					result = append(result, "#")
					break
				}
			}
		}

		if inRelevantSection {
			result = append(result, line)
		}
	}

	extracted := strings.Join(result, "\n")
	if len(extracted) > 8000 {
		extracted = extracted[:8000] + "\n...(truncated)"
	}
	return extracted
}
