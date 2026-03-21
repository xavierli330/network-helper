// Package intent provides a lightweight keyword-based classifier that maps
// natural language queries to a QueryIntent without requiring an LLM call.
// Both Chinese and English keywords are supported.
package intent

import "strings"

// QueryIntent represents the detected intent of a user query.
type QueryIntent int

const (
	// IntentComplex is the safe fallback for queries that require LLM reasoning.
	IntentComplex QueryIntent = iota
	// IntentDeviceList maps to store.ListDevices().
	IntentDeviceList
	// IntentInterfaceStatus maps to store.GetInterfaces().
	IntentInterfaceStatus
	// IntentRouteTable maps to store.GetRIB() or the scratch pad.
	IntentRouteTable
	// IntentNeighborList maps to store.GetNeighbors().
	IntentNeighborList
	// IntentConfigSearch maps to the FTS5 fts_config virtual table.
	IntentConfigSearch
)

// String returns a human-readable name for the intent, useful for debugging.
func (q QueryIntent) String() string {
	switch q {
	case IntentDeviceList:
		return "DeviceList"
	case IntentInterfaceStatus:
		return "InterfaceStatus"
	case IntentRouteTable:
		return "RouteTable"
	case IntentNeighborList:
		return "NeighborList"
	case IntentConfigSearch:
		return "ConfigSearch"
	default:
		return "Complex"
	}
}

// complexKeywords trigger an immediate IntentComplex result regardless of what
// else appears in the query. They indicate analytical or hypothetical intent.
var complexKeywords = []string{
	// Chinese
	"为什么", "是否", "分析", "诊断", "经过", "影响", "如果", "假设", "比较",
	// English
	"why", "analyze", "diagnose", "impact", "what if", "compare",
}

// actionKeywords are words that suggest a data-retrieval request.
var actionKeywords = []string{
	// Chinese
	"显示", "列出", "查看", "有哪些",
	// English
	"show", "list", "display",
}

// objectGroups maps a set of object keywords to the intent they produce when
// combined with an action keyword.
var objectGroups = []struct {
	keywords []string
	intent   QueryIntent
}{
	{
		keywords: []string{"设备", "device", "router", "switch"},
		intent:   IntentDeviceList,
	},
	{
		keywords: []string{"接口", "端口", "interface", "port"},
		intent:   IntentInterfaceStatus,
	},
	{
		keywords: []string{"路由", "路由表", "route", "routing"},
		intent:   IntentRouteTable,
	},
	{
		keywords: []string{"邻居", "neighbor", "peer"},
		intent:   IntentNeighborList,
	},
}

// searchActionKeywords are words that suggest a full-text search request.
var searchActionKeywords = []string{
	// Chinese
	"搜索", "查找",
	// English
	"search", "find", "grep",
}

// configObjectKeywords pair with searchActionKeywords to produce IntentConfigSearch.
var configObjectKeywords = []string{
	// Chinese
	"配置",
	// English
	"config",
}

// Classify maps a natural language query to a QueryIntent using keyword
// matching. It is case-insensitive for ASCII characters. When no pattern
// matches, IntentComplex is returned as a safe fallback.
func Classify(query string) QueryIntent {
	q := strings.ToLower(query)

	// Step 1 — complex keywords take immediate precedence.
	for _, kw := range complexKeywords {
		if strings.Contains(q, kw) {
			return IntentComplex
		}
	}

	// Step 2 — search action + config object → ConfigSearch.
	for _, act := range searchActionKeywords {
		if strings.Contains(q, act) {
			for _, obj := range configObjectKeywords {
				if strings.Contains(q, obj) {
					return IntentConfigSearch
				}
			}
		}
	}

	// Step 3 — retrieval action + typed object → specific intent.
	for _, act := range actionKeywords {
		if strings.Contains(q, act) {
			for _, og := range objectGroups {
				for _, obj := range og.keywords {
					if strings.Contains(q, obj) {
						return og.intent
					}
				}
			}
		}
	}

	// Step 4 — safe fallback.
	return IntentComplex
}
