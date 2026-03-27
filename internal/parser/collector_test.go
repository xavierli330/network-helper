// internal/parser/collector_test.go
package parser_test

import (
	"testing"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/parser"
	"github.com/xavierli/nethelper/internal/parser/huawei"
	"github.com/xavierli/nethelper/internal/store"
)

func TestCollectorNormalisesCommand(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()

	c := parser.NewCollector(db)
	block := parser.CommandBlock{
		Hostname: "R1", Vendor: "huawei",
		Command: "dis int brief", Output: "PHY Protocol",
	}
	if err := c.Collect(block); err != nil {
		t.Fatal(err)
	}

	rows, err := db.ListUnknownOutputs("huawei", "new", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	// Both verb and interior abbreviations are now expanded
	if rows[0].CommandNorm != "display interface brief" {
		t.Errorf("want 'display interface brief', got %q", rows[0].CommandNorm)
	}
}

func TestCollectorFiltersEmptyOutput(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	c := parser.NewCollector(db)

	// Empty output — Tab-completion intermediate, should be filtered
	block := parser.CommandBlock{
		Vendor: "huawei", Command: "display link-agg", Output: "",
	}
	c.Collect(block)

	// Whitespace-only output — also filtered
	block2 := parser.CommandBlock{
		Vendor: "huawei", Command: "display link-aggregation ver", Output: "  \n  \n",
	}
	c.Collect(block2)

	rows, _ := db.ListUnknownOutputs("huawei", "new", 10)
	if len(rows) != 0 {
		t.Errorf("want 0 rows (empty outputs filtered), got %d", len(rows))
	}
}

func TestCollectorFiltersControlCommands(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	c := parser.NewCollector(db)

	for _, cmd := range []string{"quit", "return", "system-view", "save", "screen-length 0 temporary", "sys"} {
		block := parser.CommandBlock{
			Vendor: "huawei", Command: cmd, Output: "some response",
		}
		c.Collect(block)
	}

	rows, _ := db.ListUnknownOutputs("huawei", "new", 10)
	if len(rows) != 0 {
		t.Errorf("want 0 rows (control commands filtered), got %d", len(rows))
	}
}

func TestCollectorStripsArgs(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	c := parser.NewCollector(db)

	// "display link-aggregation verbose bridge-aggregation 1" → pattern replaces trailing ID
	block := parser.CommandBlock{
		Vendor: "h3c", Command: "display link-aggregation verbose bridge-aggregation 1",
		Output: "Aggregation Interface: Bridge-Aggregation1\nSomething...",
	}
	c.Collect(block)

	rows, _ := db.ListUnknownOutputs("h3c", "new", 10)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	// "bridge-aggregation" is a qualifier keyword, "1" is its argument → {id}
	want := "display link-aggregation verbose bridge-aggregation {id}"
	if rows[0].CommandNorm != want {
		t.Errorf("want %q, got %q", want, rows[0].CommandNorm)
	}
}

func TestCollectorStripsMiddleArgs(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	c := parser.NewCollector(db)

	// VPN instance name in the middle, peer IP, and verbose modifier
	block := parser.CommandBlock{
		Vendor:  "huawei",
		Command: "display bgp vpnv4 vpn-instance CORP peer 10.0.0.1 verbose",
		Output:  "BGP Peer is 10.0.0.1...\nSome output",
	}
	c.Collect(block)

	rows, _ := db.ListUnknownOutputs("huawei", "new", 10)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	// "vpn-instance CORP" → "vpn-instance {name}", "peer 10.0.0.1" → "peer {ip}"
	want := "display bgp vpnv4 vpn-instance {name} peer {ip} verbose"
	if rows[0].CommandNorm != want {
		t.Errorf("want %q, got %q", want, rows[0].CommandNorm)
	}
}

func TestCollectorStripsInterfaceInMiddle(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	c := parser.NewCollector(db)

	// Interface name in the middle, followed by "brief" modifier
	block := parser.CommandBlock{
		Vendor:  "huawei",
		Command: "display interface GigabitEthernet0/0/1 brief",
		Output:  "Interface  PHY  Protocol\nGE0/0/1  up  up",
	}
	c.Collect(block)

	rows, _ := db.ListUnknownOutputs("huawei", "new", 10)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	// "interface" is a qualifier keyword, next word (interface name) → {interface}
	want := "display interface {interface} brief"
	if rows[0].CommandNorm != want {
		t.Errorf("want %q, got %q", want, rows[0].CommandNorm)
	}
}

func TestCollectorFiltersHelpEcho(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	c := parser.NewCollector(db)

	// Typical help echo output containing <cr> and redirect hints
	helpOutput := `  display       Display information
  interface     Select an interface to configure
  <cr>
  >             Redirect it to a file
  |             Matching output`

	block := parser.CommandBlock{
		Vendor: "huawei", Command: "display interface",
		Output: helpOutput,
	}
	c.Collect(block)

	rows, _ := db.ListUnknownOutputs("huawei", "new", 10)
	if len(rows) != 0 {
		t.Errorf("want 0 rows (help echo filtered), got %d", len(rows))
	}
}

func TestCollectorFiltersHelpEchoByCR(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	c := parser.NewCollector(db)

	// Minimal help echo — just has <cr>
	helpOutput := `  bridge-aggregation  Show bridge aggregation info
  <cr>`

	block := parser.CommandBlock{
		Vendor: "h3c", Command: "display link-aggregation verbose",
		Output: helpOutput,
	}
	c.Collect(block)

	rows, _ := db.ListUnknownOutputs("h3c", "new", 10)
	if len(rows) != 0 {
		t.Errorf("want 0 rows (help echo with <cr> filtered), got %d", len(rows))
	}
}

func TestCollectorExpandsInteriorAbbrevs(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	c := parser.NewCollector(db)

	block := parser.CommandBlock{
		Vendor: "h3c", Command: "dis link-agg ver bridge-aggregation 2",
		Output: "Aggregation Interface: Bridge-Aggregation2\nData...",
	}
	c.Collect(block)

	rows, _ := db.ListUnknownOutputs("h3c", "new", 10)
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	// "link-agg" → "link-aggregation", "ver" → "verbose", "2" → "{id}"
	want := "display link-aggregation verbose bridge-aggregation {id}"
	if rows[0].CommandNorm != want {
		t.Errorf("want %q, got %q", want, rows[0].CommandNorm)
	}
}

func TestCollectorDeduplicates(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	c := parser.NewCollector(db)

	block := parser.CommandBlock{
		Vendor: "huawei", Command: "display traffic-policy", Output: "identical output",
	}
	c.Collect(block)
	c.Collect(block) // same content

	rows, _ := db.ListUnknownOutputs("huawei", "new", 10)
	if len(rows) != 1 {
		t.Errorf("want 1 deduplicated row, got %d", len(rows))
	}
	if rows[0].OccurrenceCount != 2 {
		t.Errorf("want occurrence_count=2, got %d", rows[0].OccurrenceCount)
	}
}

func TestCollectorSilentOnNilDB(t *testing.T) {
	c := parser.NewCollector(nil)
	block := parser.CommandBlock{Vendor: "huawei", Command: "display foo", Output: "x"}
	// Must not panic; error is silently swallowed
	c.Collect(block)
}

func TestPipelineCollectsUnknownCommands(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()

	reg := parser.NewRegistry()
	reg.Register(huawei.New())
	pipe := parser.NewPipelineWithCollector(db, reg, parser.NewCollector(db))

	// Hostname must be ≥3 chars — huawei.DetectPrompt rejects shorter hostnames
	// "display traffic-policy" is not in huawei.ClassifyCommand → CmdUnknown
	log := "<RTR>display traffic-policy\nPolicy: p1\n"
	_, err := pipe.Ingest("test.log", log)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := db.ListUnknownOutputs("huawei", "new", 10)
	if len(rows) != 1 {
		t.Errorf("want 1 unknown output collected, got %d", len(rows))
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// New qualifier keywords tests (Section 6.2 of analysis report)
// ═══════════════════════════════════════════════════════════════════════════

func TestCollectorStripsNewQualifierKeywords(t *testing.T) {
	tests := []struct {
		name    string
		vendor  string
		command string
		want    string
	}{
		// Cisco location
		{"cisco_location", "cisco", "show processes cpu location 0/0/CPU0", "show processes cpu location {name}"},
		// Cisco bundle-ether
		{"cisco_bundle_ether", "cisco", "show bundle bundle-ether 10", "show bundle bundle-ether {id}"},
		// Cisco rd
		{"cisco_rd", "cisco", "show bgp vpnv4 unicast rd 65000:100", "show bgp vpnv4 unicast rd {name}"},
		// Cisco prefix-set
		{"cisco_prefix_set", "cisco", "show rpl prefix-set CUSTOMER_ROUTES", "show rpl prefix-set {name}"},
		// Juniper routing-instance
		{"juniper_routing_instance", "juniper", "show route routing-instance VPN_A", "show route routing-instance {name}"},
		// Juniper policy-statement
		{"juniper_policy_statement", "juniper", "show policy-options policy-statement EXPORT_POLICY", "show policy-options policy-statement {name}"},
		// 华为 ip-prefix
		{"huawei_ip_prefix", "huawei", "display ip ip-prefix CUSTOMER", "display ip ip-prefix {name}"},
		// 华为 community-filter
		{"huawei_community_filter", "huawei", "display ip community-filter COMM1", "display ip community-filter {name}"},
		// 华为 as-path-filter
		{"huawei_as_path_filter", "huawei", "display ip as-path-filter 10", "display ip as-path-filter {id}"},
		// 华为 tunnel-policy
		{"huawei_tunnel_policy", "huawei", "display tunnel-policy TP_VPN", "display tunnel-policy {name}"},
		// 华为 configuration section
		{"huawei_configuration", "huawei", "display current-configuration configuration bgp", "display current-configuration configuration {name}"},
		// MPLS label
		{"juniper_label", "juniper", "show route table mpls.0 label 100200", "show route table {name} label {id}"},
		// destination
		{"huawei_destination", "huawei", "display tunnel-info destination 10.0.0.1", "display tunnel-info destination {ip}"},
		// source (ping)
		{"cisco_source", "cisco", "ping 10.0.0.1 source 192.168.1.1", "ping {ip} source {ip}"},
		// Juniper filter
		{"juniper_filter", "juniper", "show firewall filter PROTECT_RE", "show firewall filter {name}"},
		// Cisco last
		{"cisco_last", "cisco", "show logging last 100", "show logging last {id}"},
		// SR-TE policy name
		{"huawei_sr_te_name", "huawei", "display segment-routing te policy name POLICY_TO_PE2", "display segment-routing te policy name {name}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := store.Open(":memory:")
			defer db.Close()
			c := parser.NewCollector(db)
			block := parser.CommandBlock{
				Vendor: tt.vendor, Command: tt.command,
				Output:   "some output data\nmore data",
				Hostname: "R1",
			}
			c.Collect(block)
			rows, _ := db.ListUnknownOutputs(tt.vendor, "new", 10)
			if len(rows) != 1 {
				t.Fatalf("want 1 row, got %d", len(rows))
			}
			if rows[0].CommandNorm != tt.want {
				t.Errorf("want %q, got %q", tt.want, rows[0].CommandNorm)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Help echo / error output detection tests (Section 7 of analysis report)
// ═══════════════════════════════════════════════════════════════════════════

func TestCollectorFiltersJuniperHelp(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	c := parser.NewCollector(db)

	juniperHelp := "Possible completions:\n  <prefix>          IP prefix\n  detail            Show detailed output\n  extensive         Show extensive output"
	block := parser.CommandBlock{
		Vendor: "juniper", Command: "show route",
		Output: juniperHelp, Hostname: "MX1",
	}
	c.Collect(block)
	rows, _ := db.ListUnknownOutputs("juniper", "new", 10)
	if len(rows) != 0 {
		t.Errorf("want 0 rows (Juniper help filtered), got %d", len(rows))
	}
}

func TestCollectorFiltersCiscoError(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	c := parser.NewCollector(db)

	ciscoError := "% Invalid input detected at '^' marker.\n\n  show bgpp\n       ^"
	block := parser.CommandBlock{
		Vendor: "cisco", Command: "show bgpp",
		Output: ciscoError, Hostname: "ASR1",
	}
	c.Collect(block)
	rows, _ := db.ListUnknownOutputs("cisco", "new", 10)
	if len(rows) != 0 {
		t.Errorf("want 0 rows (Cisco error filtered), got %d", len(rows))
	}
}

func TestCollectorFiltersHuaweiError(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()
	c := parser.NewCollector(db)

	hwError := "Error: Unrecognized command found at '^' position."
	block := parser.CommandBlock{
		Vendor: "huawei", Command: "display bgpp",
		Output: hwError, Hostname: "NE40",
	}
	c.Collect(block)
	rows, _ := db.ListUnknownOutputs("huawei", "new", 10)
	if len(rows) != 0 {
		t.Errorf("want 0 rows (Huawei error filtered), got %d", len(rows))
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Modifier keywords preservation tests (Section 8.2 of analysis report)
// ═══════════════════════════════════════════════════════════════════════════

func TestCollectorPreservesNewModifiers(t *testing.T) {
	tests := []struct {
		name    string
		vendor  string
		command string
		want    string
	}{
		{"all_modifier", "huawei", "display bgp vpnv4 all routing-table", "display bgp vpnv4 all routing-table"},
		{"active_modifier", "juniper", "show route active-path", "show route active-path"},
		{"exact_modifier", "juniper", "show route 10.0.0.0/24 exact", "show route {ip} exact"},
		{"external_modifier", "huawei", "display ospf lsdb external", "display ospf lsdb external"},
		{"inbound_outbound", "huawei", "display traffic-policy statistics interface gigabitethernet0/0/1 inbound", "display traffic-policy statistics interface {interface} inbound"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := store.Open(":memory:")
			defer db.Close()
			c := parser.NewCollector(db)
			block := parser.CommandBlock{
				Vendor: tt.vendor, Command: tt.command,
				Output: "output data", Hostname: "R1",
			}
			c.Collect(block)
			rows, _ := db.ListUnknownOutputs(tt.vendor, "new", 10)
			if len(rows) != 1 {
				t.Fatalf("want 1 row, got %d", len(rows))
			}
			if rows[0].CommandNorm != tt.want {
				t.Errorf("want %q, got %q", tt.want, rows[0].CommandNorm)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Interior abbreviation expansion tests (Section 8.7 of analysis report)
// ═══════════════════════════════════════════════════════════════════════════

func TestCollectorExpandsNewAbbrevs(t *testing.T) {
	tests := []struct {
		name    string
		vendor  string
		command string
		want    string
	}{
		{"nei_abbrev", "cisco", "sh bgp nei 10.0.0.1", "show bgp neighbor {ip}"},
		{"neigh_abbrev", "cisco", "sh bgp neigh 10.0.0.2", "show bgp neighbor {ip}"},
		{"stat_abbrev", "huawei", "dis int gigabitethernet0/0/1 stat", "display interface {interface} statistics"},
		{"desc_abbrev", "juniper", "show interfaces desc", "show interfaces descriptions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := store.Open(":memory:")
			defer db.Close()
			c := parser.NewCollector(db)
			block := parser.CommandBlock{
				Vendor: tt.vendor, Command: tt.command,
				Output: "output data", Hostname: "R1",
			}
			c.Collect(block)
			rows, _ := db.ListUnknownOutputs(tt.vendor, "new", 10)
			if len(rows) != 1 {
				t.Fatalf("want 1 row, got %d", len(rows))
			}
			if rows[0].CommandNorm != tt.want {
				t.Errorf("want %q, got %q", tt.want, rows[0].CommandNorm)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// Command relation data structure tests (Section 8.4 of analysis report)
// ═══════════════════════════════════════════════════════════════════════════

func TestCommandRelationsNotEmpty(t *testing.T) {
	// 验证核心关联数据已定义
	if len(model.CoreCommandRelations) == 0 {
		t.Error("CoreCommandRelations should not be empty")
	}
	// 验证每个关联的必填字段
	for i, rel := range model.CoreCommandRelations {
		if rel.SourceCmd == "" || rel.TargetCmd == "" {
			t.Errorf("relation[%d]: SourceCmd and TargetCmd must not be empty", i)
		}
		if rel.RelationType == "" {
			t.Errorf("relation[%d]: RelationType must not be empty", i)
		}
	}
}

func TestTroubleshootScenariosNotEmpty(t *testing.T) {
	if len(model.CoreTroubleshootScenarios) == 0 {
		t.Error("CoreTroubleshootScenarios should not be empty")
	}
	for i, sc := range model.CoreTroubleshootScenarios {
		if sc.ID == "" || sc.Name == "" {
			t.Errorf("scenario[%d]: ID and Name must not be empty", i)
		}
		if len(sc.Steps) == 0 {
			t.Errorf("scenario[%d] %s: must have at least one step", i, sc.ID)
		}
	}
}
