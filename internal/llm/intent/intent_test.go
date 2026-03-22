package intent_test

import (
	"testing"

	"github.com/xavierli/nethelper/internal/llm/intent"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  intent.QueryIntent
	}{
		// --- Chinese queries ---
		{
			name:  "chinese device list",
			query: "查看所有设备",
			want:  intent.IntentDeviceList,
		},
		{
			name:  "chinese interface status",
			query: "列出接口状态",
			want:  intent.IntentInterfaceStatus,
		},
		{
			name:  "chinese BGP neighbor list",
			query: "显示BGP邻居",
			want:  intent.IntentNeighborList,
		},
		{
			name:  "chinese route table",
			query: "显示路由表",
			want:  intent.IntentRouteTable,
		},
		{
			name:  "chinese config search",
			query: "搜索配置中的acl",
			want:  intent.IntentConfigSearch,
		},

		// --- English queries ---
		{
			name:  "english device list",
			query: "show all devices",
			want:  intent.IntentDeviceList,
		},
		{
			name:  "english interface list with device",
			query: "list interfaces on core-01",
			want:  intent.IntentInterfaceStatus,
		},
		{
			name:  "english route table",
			query: "display routing table",
			want:  intent.IntentRouteTable,
		},
		{
			name:  "english config search",
			query: "find config for bgp",
			want:  intent.IntentConfigSearch,
		},
		{
			name:  "english grep config",
			query: "grep config ospf",
			want:  intent.IntentConfigSearch,
		},

		// --- Mixed / natural-language queries ---
		{
			name:  "mixed BGP neighbor with device name",
			query: "show me the BGP neighbors for gz-hxy",
			want:  intent.IntentNeighborList,
		},
		{
			name:  "list all routers",
			query: "list all routers in the network",
			want:  intent.IntentDeviceList,
		},
		{
			name:  "display interfaces uppercase",
			query: "Display Interfaces for R1",
			want:  intent.IntentInterfaceStatus,
		},
		{
			name:  "show route",
			query: "show route to 10.0.0.1",
			want:  intent.IntentRouteTable,
		},
		{
			name:  "show peer",
			query: "show peer summary",
			want:  intent.IntentNeighborList,
		},

		// --- Complex / analytical queries ---
		{
			name:  "chinese why OSPF down",
			query: "为什么OSPF邻居断了",
			want:  intent.IntentComplex,
		},
		{
			name:  "chinese analyze route",
			query: "分析这个路由",
			want:  intent.IntentComplex,
		},
		{
			name:  "english diagnose",
			query: "diagnose the BGP session",
			want:  intent.IntentComplex,
		},
		{
			name:  "english why",
			query: "why is the interface down",
			want:  intent.IntentComplex,
		},
		{
			name:  "english what if",
			query: "what if link fails",
			want:  intent.IntentComplex,
		},
		{
			name:  "english compare configs",
			query: "compare configs between two snapshots",
			want:  intent.IntentComplex,
		},
		{
			name:  "chinese impact",
			query: "影响有哪些设备",
			// "影响" is a complex keyword, even though "有哪些设备" would map to DeviceList
			want: intent.IntentComplex,
		},

		// --- Edge cases ---
		{
			name:  "empty string",
			query: "",
			want:  intent.IntentComplex,
		},
		{
			name:  "gibberish",
			query: "asdfghjkl",
			want:  intent.IntentComplex,
		},
		{
			name:  "only action no object",
			query: "show",
			want:  intent.IntentComplex,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := intent.Classify(tc.query)
			if got != tc.want {
				t.Errorf("Classify(%q) = %s, want %s", tc.query, got, tc.want)
			}
		})
	}
}

func TestQueryIntentString(t *testing.T) {
	cases := []struct {
		intent intent.QueryIntent
		want   string
	}{
		{intent.IntentComplex, "Complex"},
		{intent.IntentDeviceList, "DeviceList"},
		{intent.IntentInterfaceStatus, "InterfaceStatus"},
		{intent.IntentRouteTable, "RouteTable"},
		{intent.IntentNeighborList, "NeighborList"},
		{intent.IntentConfigSearch, "ConfigSearch"},
		// Out-of-range value should still return "Complex"
		{intent.QueryIntent(999), "Complex"},
	}

	for _, tc := range cases {
		got := tc.intent.String()
		if got != tc.want {
			t.Errorf("QueryIntent(%d).String() = %q, want %q", tc.intent, got, tc.want)
		}
	}
}
