package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/llm"
	"github.com/xavierli/nethelper/internal/llm/intent"
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

			// Classify intent — simple queries bypass LLM entirely.
			queryIntent := intent.Classify(userQuestion)
			if queryIntent != intent.IntentComplex {
				result := directQuery(queryIntent, userQuestion, deviceID)
				if result != "" {
					fmt.Println(result)
					return nil
				}
				// Fall through to LLM if directQuery couldn't answer.
			}

			if !llmRouter.Available() {
				fmt.Println("No LLM provider configured.")
				fmt.Println("Configure an LLM: nethelper config llm --help")
				return nil
			}

			// Gather intent-aware context from nethelper memory.
			ctx := buildContext(userQuestion, queryIntent, deviceID)

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

// directQuery attempts to answer simple intents directly from the DB without
// invoking an LLM. It returns the formatted answer string, or "" if it cannot
// satisfy the query (caller should fall through to LLM).
func directQuery(qi intent.QueryIntent, question, deviceID string) string {
	switch qi {
	case intent.IntentDeviceList:
		devices, err := db.ListDevices()
		if err != nil || len(devices) == 0 {
			return ""
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%-30s %-20s %-12s %-16s\n", "DEVICE ID", "HOSTNAME", "VENDOR", "ROUTER-ID"))
		sb.WriteString(strings.Repeat("-", 82) + "\n")
		for _, d := range devices {
			sb.WriteString(fmt.Sprintf("%-30s %-20s %-12s %-16s\n",
				d.ID, d.Hostname, d.Vendor, d.RouterID))
		}
		return sb.String()

	case intent.IntentInterfaceStatus:
		targets := findRelevantDevices(question, deviceID)
		if len(targets) == 0 {
			return ""
		}
		var sb strings.Builder
		for _, devID := range targets {
			ifaces, err := db.GetInterfaces(devID)
			if err != nil || len(ifaces) == 0 {
				continue
			}
			sb.WriteString(fmt.Sprintf("=== %s ===\n", devID))
			sb.WriteString(fmt.Sprintf("%-30s %-10s %-8s %-20s %s\n", "INTERFACE", "TYPE", "STATUS", "IP/MASK", "DESCRIPTION"))
			sb.WriteString(strings.Repeat("-", 90) + "\n")
			for _, i := range ifaces {
				ip := i.IPAddress
				if ip != "" && i.Mask != "" {
					ip = ip + "/" + i.Mask
				}
				sb.WriteString(fmt.Sprintf("%-30s %-10s %-8s %-20s %s\n",
					i.Name, string(i.Type), i.Status, ip, i.Description))
			}
			sb.WriteString("\n")
		}
		result := strings.TrimRight(sb.String(), "\n")
		if result == "" {
			return ""
		}
		return result

	case intent.IntentRouteTable:
		targets := findRelevantDevices(question, deviceID)
		if len(targets) == 0 {
			return ""
		}
		var sb strings.Builder
		for _, devID := range targets {
			entries, err := db.ListScratch(devID, "route", 5)
			if err != nil || len(entries) == 0 {
				continue
			}
			sb.WriteString(fmt.Sprintf("=== %s — 最近路由表输出 ===\n", devID))
			for _, e := range entries {
				sb.WriteString(fmt.Sprintf("--- [%s] %s ---\n", e.Category, e.Query))
				content := e.Content
				if len(content) > 2000 {
					content = content[:2000] + "\n...(truncated)"
				}
				sb.WriteString(content + "\n\n")
			}
		}
		result := strings.TrimRight(sb.String(), "\n")
		if result == "" {
			return ""
		}
		return result

	case intent.IntentNeighborList:
		targets := findRelevantDevices(question, deviceID)
		if len(targets) == 0 {
			return ""
		}
		var sb strings.Builder
		for _, devID := range targets {
			snapID, err := db.LatestSnapshotID(devID)
			if err != nil {
				continue
			}
			neighbors, err := db.GetNeighbors(devID, snapID)
			if err != nil || len(neighbors) == 0 {
				continue
			}
			sb.WriteString(fmt.Sprintf("=== %s ===\n", devID))
			sb.WriteString(fmt.Sprintf("%-8s %-20s %-12s %-20s %-8s %-6s %s\n",
				"PROTO", "REMOTE-ID", "STATE", "INTERFACE", "AREA", "AS", "UPTIME"))
			sb.WriteString(strings.Repeat("-", 90) + "\n")
			for _, n := range neighbors {
				sb.WriteString(fmt.Sprintf("%-8s %-20s %-12s %-20s %-8s %-6d %s\n",
					n.Protocol, n.RemoteID, n.State, n.LocalInterface,
					n.AreaID, n.ASNumber, n.Uptime))
			}
			sb.WriteString("\n")
		}
		result := strings.TrimRight(sb.String(), "\n")
		if result == "" {
			return ""
		}
		return result

	case intent.IntentConfigSearch:
		term := extractSearchTerm(question)
		if term == "" {
			return ""
		}
		configs, err := db.SearchConfig(term)
		if err != nil || len(configs) == 0 {
			return ""
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("搜索 \"%s\" 的配置结果 (%d 条):\n\n", term, len(configs)))
		for _, c := range configs {
			sb.WriteString(fmt.Sprintf("--- 设备: %s (捕获于 %s) ---\n```\n", c.DeviceID, c.CapturedAt.Format("2006-01-02 15:04")))
			text := c.ConfigText
			if len(text) > 1500 {
				text = text[:1500] + "\n...(truncated)"
			}
			sb.WriteString(text + "\n```\n\n")
		}
		return strings.TrimRight(sb.String(), "\n")
	}

	return ""
}

// extractSearchTerm pulls the search term from a config search query.
// e.g. "search config bgp" → "bgp", "查找配置 ospf" → "ospf"
func extractSearchTerm(question string) string {
	lower := strings.ToLower(strings.TrimSpace(question))
	// Remove leading action keywords and config keywords, use the remainder.
	for _, kw := range []string{"search config", "find config", "grep config",
		"搜索配置", "查找配置", "search", "find", "grep", "搜索", "查找",
		"config", "配置"} {
		lower = strings.ReplaceAll(lower, kw, " ")
	}
	term := strings.TrimSpace(lower)
	// Return first non-empty token as search term.
	parts := strings.Fields(term)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// buildContext collects relevant data from the nethelper database.
// It is intent-aware: protocol-specific questions get filtered context instead
// of dumping all interfaces and all neighbors.
func buildContext(question string, qi intent.QueryIntent, explicitDevice string) string {
	var sections []string

	// Determine target device(s).
	targetDevices := findRelevantDevices(question, explicitDevice)

	if len(targetDevices) == 0 {
		// No specific device found — provide general overview.
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

	// Determine which protocol filter to apply based on the question.
	protoFilter := detectProtocolFilter(question)

	for _, devID := range targetDevices {
		dev, err := db.GetDevice(devID)
		if err != nil {
			continue
		}

		sections = append(sections, fmt.Sprintf("## 设备: %s (hostname=%s, vendor=%s, router_id=%s, mgmt_ip=%s)",
			dev.ID, dev.Hostname, dev.Vendor, dev.RouterID, dev.MgmtIP))

		// Interfaces — skip or filter based on protocol context.
		if protoFilter == "" {
			// General query: include all interfaces.
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
		}
		// For protocol-specific queries we skip the full interface dump to save tokens.

		// Neighbors — filter by protocol when applicable.
		snapID, err := db.LatestSnapshotID(devID)
		if err == nil {
			neighbors, _ := db.GetNeighbors(devID, snapID)
			if len(neighbors) > 0 {
				var nLines []string
				nLines = append(nLines, "### 协议邻居")
				for _, n := range neighbors {
					// When a protocol filter is active, skip unrelated neighbors.
					if protoFilter != "" && !strings.Contains(strings.ToLower(n.Protocol), protoFilter) {
						continue
					}
					nLines = append(nLines, fmt.Sprintf("- protocol=%s remote_id=%s state=%s interface=%s area=%s as=%d uptime=%s",
						n.Protocol, n.RemoteID, n.State, n.LocalInterface, n.AreaID, n.ASNumber, n.Uptime))
				}
				if len(nLines) > 1 { // more than just the header
					sections = append(sections, strings.Join(nLines, "\n"))
				}
			}
		}

		// Config snapshot — intent-aware extraction and cap.
		configs, _ := db.GetConfigSnapshots(devID)
		if len(configs) > 0 {
			configText := configs[0].ConfigText
			var maxChars int
			if protoFilter != "" {
				maxChars = 4000 // protocol-specific
			} else {
				maxChars = 5000 // general
			}
			relevantConfig := extractRelevantConfig(configText, question, maxChars)
			if relevantConfig != "" {
				sections = append(sections, "### 相关配置片段\n```\n"+relevantConfig+"\n```")
			} else if len(configText) > 3000 {
				sections = append(sections, "### 配置摘要（前3000字符）\n```\n"+configText[:3000]+"\n...(truncated)\n```")
			} else {
				sections = append(sections, "### 完整配置\n```\n"+configText+"\n```")
			}
		}

		// Scratch pad entries — filter category by protocol filter if possible.
		scratchCat := ""
		if protoFilter == "mpls" || protoFilter == "tunnel" {
			scratchCat = "label"
		}
		scratches, _ := db.ListScratch(devID, scratchCat, 5)
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

// detectProtocolFilter inspects the question and returns a protocol name
// ("ospf", "bgp", "mpls", "tunnel", "") that should be used to filter
// neighbors and config sections. Returns "" for general queries.
func detectProtocolFilter(question string) string {
	lower := strings.ToLower(question)
	switch {
	case strings.Contains(lower, "ospf"):
		return "ospf"
	case strings.Contains(lower, "bgp"):
		return "bgp"
	case strings.Contains(lower, "mpls") || strings.Contains(lower, " ldp") ||
		strings.Contains(lower, " rsvp") || strings.Contains(lower, "segment-routing") ||
		strings.Contains(lower, " sr "):
		return "mpls"
	case strings.Contains(lower, "tunnel") || strings.Contains(lower, " te ") ||
		strings.HasSuffix(lower, " te"):
		return "tunnel"
	default:
		return ""
	}
}


// findRelevantDevices identifies which device(s) the question is about.
func findRelevantDevices(question, explicitDevice string) []string {
	if explicitDevice != "" {
		return []string{strings.ToLower(explicitDevice)}
	}

	// Try to match device IDs or hostnames mentioned in the question.
	devices, err := db.ListDevices()
	if err != nil {
		return nil
	}

	lower := strings.ToLower(question)
	var matches []string
	for _, d := range devices {
		if strings.Contains(lower, strings.ToLower(d.ID)) ||
			strings.Contains(lower, strings.ToLower(d.Hostname)) {
			matches = append(matches, d.ID)
		}
	}

	// If no device matched but there's only one device in DB, use it.
	if len(matches) == 0 && len(devices) == 1 {
		matches = []string{devices[0].ID}
	}

	return matches
}

// extractRelevantConfig extracts config sections relevant to the question.
// maxChars limits the returned string length.
func extractRelevantConfig(fullConfig, question string, maxChars int) string {
	lower := strings.ToLower(question)

	// Keywords to config section headers.
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
		"接口":      {"interface"},
		"邻居":      {"bgp", "ospf", "isis"},
		"路由":      {"route-policy", "bgp", "ospf"},
		"策略":      {"route-policy", "traffic-policy", "acl"},
	}

	var sectionNames []string
	for keyword, sections := range keywords {
		if strings.Contains(lower, keyword) {
			sectionNames = append(sectionNames, sections...)
		}
	}

	if len(sectionNames) == 0 {
		return ""
	}

	// Extract matching sections (Huawei uses # as section delimiter).
	lines := strings.Split(fullConfig, "\n")
	var result []string
	inRelevantSection := false
	sectionDepth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "#" {
			if inRelevantSection && sectionDepth == 0 {
				result = append(result, line)
				inRelevantSection = false
				continue
			}
			inRelevantSection = false
			sectionDepth = 0
		}

		if !inRelevantSection {
			lineLower := strings.ToLower(trimmed)
			for _, name := range sectionNames {
				if strings.Contains(lineLower, name) {
					inRelevantSection = true
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
	if len(extracted) > maxChars {
		extracted = extracted[:maxChars] + "\n...(truncated)"
	}
	return extracted
}
