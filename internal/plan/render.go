package plan

import (
	"fmt"
	"strings"
)

// RenderText produces a plain-text representation of a Plan suitable for
// display in a terminal or plain-text log file.
func RenderText(p Plan) string {
	var sb strings.Builder

	// Header
	sb.WriteString("=== 设备隔离变更方案 ===\n")
	sb.WriteString(fmt.Sprintf("目标设备: %s (%s)\n", p.TargetHostname, p.TargetVendor))
	sb.WriteString(fmt.Sprintf("生成时间: %s\n", p.GeneratedAt.Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("互联设备: %d 台\n", len(uniquePeers(p.Links))))

	if p.IsSPOF {
		sb.WriteString(fmt.Sprintf("影响评估: ⚠️ SPOF — 移除后 %d 台设备受影响\n", len(p.ImpactDevices)))
	}

	// Interconnection table
	if len(p.Links) > 0 {
		sb.WriteString("\n互联关系:\n")
		for _, l := range p.Links {
			sources := strings.Join(l.Sources, ",")
			protocols := strings.Join(l.Protocols, ",")
			sb.WriteString(fmt.Sprintf("  %s %s  <-->  %s %s  (来源: %s, 协议: %s)\n",
				l.LocalDevice, l.LocalInterface,
				l.PeerDevice, l.PeerInterface,
				sources, protocols,
			))
		}
	}

	// Phases
	for _, phase := range p.Phases {
		sb.WriteString(fmt.Sprintf("\n─── 阶段%d: %s ───\n", phase.Number, phase.Name))
		if phase.Description != "" {
			sb.WriteString(fmt.Sprintf("  %s\n", phase.Description))
		}
		for _, note := range phase.Notes {
			sb.WriteString(fmt.Sprintf("  > %s\n", note))
		}
		for _, step := range phase.Steps {
			sb.WriteString(fmt.Sprintf("  [%s] %s\n", step.DeviceID, step.Purpose))
			for _, cmd := range step.Commands {
				sb.WriteString(fmt.Sprintf("    %s\n", cmd))
			}
		}
	}

	return sb.String()
}

// RenderMarkdown produces a Markdown representation of a Plan suitable for
// inclusion in documentation, Confluence pages, or chat messages.
func RenderMarkdown(p Plan) string {
	var sb strings.Builder

	// Title
	sb.WriteString("# 设备隔离变更方案\n\n")

	// Meta table
	sb.WriteString("| 字段 | 值 |\n")
	sb.WriteString("|------|----|\n")
	sb.WriteString(fmt.Sprintf("| 目标设备 | %s (%s) |\n", p.TargetHostname, p.TargetVendor))
	sb.WriteString(fmt.Sprintf("| 生成时间 | %s |\n", p.GeneratedAt.Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("| 互联设备 | %d 台 |\n", len(uniquePeers(p.Links))))

	if p.IsSPOF {
		sb.WriteString(fmt.Sprintf("| 影响评估 | ⚠️ SPOF — 移除后 %d 台设备受影响 |\n", len(p.ImpactDevices)))
	}
	sb.WriteString("\n")

	// Interconnection table
	if len(p.Links) > 0 {
		sb.WriteString("## 互联关系\n\n")
		sb.WriteString("| 本端设备 | 本端接口 | 对端设备 | 对端接口 | 来源 | 协议 |\n")
		sb.WriteString("|----------|----------|----------|----------|------|------|\n")
		for _, l := range p.Links {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
				l.LocalDevice, l.LocalInterface,
				l.PeerDevice, l.PeerInterface,
				strings.Join(l.Sources, ", "),
				strings.Join(l.Protocols, ", "),
			))
		}
		sb.WriteString("\n")
	}

	// Phases
	for _, phase := range p.Phases {
		sb.WriteString(fmt.Sprintf("## 阶段%d: %s\n\n", phase.Number, phase.Name))
		if phase.Description != "" {
			sb.WriteString(fmt.Sprintf("%s\n\n", phase.Description))
		}
		if len(phase.Notes) > 0 {
			sb.WriteString("**注意事项:**\n\n")
			for _, note := range phase.Notes {
				sb.WriteString(fmt.Sprintf("- %s\n", note))
			}
			sb.WriteString("\n")
		}
		for _, step := range phase.Steps {
			sb.WriteString(fmt.Sprintf("### [%s] %s\n\n", step.DeviceID, step.Purpose))
			sb.WriteString("```\n")
			for _, cmd := range step.Commands {
				sb.WriteString(cmd + "\n")
			}
			sb.WriteString("```\n\n")
		}
	}

	return sb.String()
}
