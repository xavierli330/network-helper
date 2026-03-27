package engine

import (
	"encoding/json"
	"testing"
)

// ── Test: Basic SPLIT (H3C show ip interface brief) ──────────────────────

func TestExecPipeline_SplitBasic(t *testing.T) {
	dsl := `
SKIP_UNTIL  ^Interface\s+Physical
SPLIT       $interface $physical $protocol $ip $description
`
	raw := `*down: administratively down
(s): spoofing
Interface                     Physical  Protocol  IP Address      Description
FGE2/0/1                      up        up        10.48.58.244    SH-YQ-050-NE40E
FGE2/0/2                      up        up        10.48.58.246    SH-YQ-050-NE40E
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if result.Mode != "table" {
		t.Errorf("expected mode=table, got %q", result.Mode)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}

	// Row 0
	assertField(t, result.Rows[0], "interface", "FGE2/0/1")
	assertField(t, result.Rows[0], "physical", "up")
	assertField(t, result.Rows[0], "protocol", "up")
	assertField(t, result.Rows[0], "ip", "10.48.58.244")
	assertField(t, result.Rows[0], "description", "SH-YQ-050-NE40E")

	// Row 1
	assertField(t, result.Rows[1], "interface", "FGE2/0/2")
	assertField(t, result.Rows[1], "ip", "10.48.58.246")

	// Columns order
	expectedCols := []string{"interface", "physical", "protocol", "ip", "description"}
	if len(result.Columns) != len(expectedCols) {
		t.Errorf("expected %d columns, got %d", len(expectedCols), len(result.Columns))
	}
	for i, c := range expectedCols {
		if i < len(result.Columns) && result.Columns[i] != c {
			t.Errorf("column[%d]: expected %q, got %q", i, c, result.Columns[i])
		}
	}
}

// ── Test: Multi-word description captured by last SPLIT variable ─────────

func TestExecPipeline_SplitLastVarCapturesRest(t *testing.T) {
	dsl := `SKIP_UNTIL ^Interface\s+Physical
SPLIT $interface $physical $protocol $ip $description`

	raw := `Interface                     Physical  Protocol  IP Address      Description
FGE2/0/1                      up        up        10.48.58.244    SH-YQ-050-NE40E to PE2
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	// "SH-YQ-050-NE40E to PE2" should be captured as one value
	assertField(t, result.Rows[0], "description", "SH-YQ-050-NE40E to PE2")
}

// ── Test: Huawei display interface brief ─────────────────────────────────

func TestExecPipeline_HuaweiInterfaceBrief(t *testing.T) {
	dsl := `
SKIP_UNTIL  ^Interface\s+PHY
SPLIT       $interface $phy $protocol $in_util $out_util $in_errors $out_errors
`
	raw := `Interface         PHY      Protocol  InUti  OutUti  inErrors outErrors
GigabitEthernet0/0/0  up       up        0.01%  0.01%       0        0
GigabitEthernet0/0/1  down     down         --     --       0        0
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}
	assertField(t, result.Rows[0], "interface", "GigabitEthernet0/0/0")
	assertField(t, result.Rows[0], "phy", "up")
	assertField(t, result.Rows[0], "in_util", "0.01%")
	assertField(t, result.Rows[1], "phy", "down")
	assertField(t, result.Rows[1], "in_util", "--")
}

// ── Test: BGP peer with SKIP_UNTIL and blank line handling ───────────────

func TestExecPipeline_BGPPeer(t *testing.T) {
	dsl := `
SKIP_UNTIL  ^\s+Peer\s+V\s+AS
SKIP_BLANK
SPLIT       $peer $version $as $msg_rcvd $msg_sent $outq $uptime $state $pref_rcv
`
	raw := `BGP local router ID : 10.0.0.1
Local AS number : 100

Total peers : 3
Peers in established state : 2

  Peer            V    AS  MsgRcvd  MsgSent  OutQ  Up/Down       State PrefRcv
  10.0.0.2        4   100     1234     5678     0 2d03h       Established     15
  10.0.0.3        4   200      567      890     0 01:23:45    Active           0
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}
	assertField(t, result.Rows[0], "peer", "10.0.0.2")
	assertField(t, result.Rows[0], "version", "4")
	assertField(t, result.Rows[0], "as", "100")
	assertField(t, result.Rows[0], "state", "Established")
	assertField(t, result.Rows[0], "pref_rcv", "15")
	assertField(t, result.Rows[1], "peer", "10.0.0.3")
	assertField(t, result.Rows[1], "state", "Active")
}

// ── Test: Record mode (REGEX only) ───────────────────────────────────────

func TestExecPipeline_RecordMode(t *testing.T) {
	dsl := `
REGEX   ^(?P<interface>\S+)\s+current state\s*:\s*(?P<status>\S+)
REGEX   ^Line protocol current state\s*:\s*(?P<protocol>\S+)
REGEX   ^Description:\s*(?P<description>.+)
REGEX   ^Input:\s+(?P<in_packets>\d+)\s+packets.*?(?P<in_bytes>\d+)\s+bytes
REGEX   ^Output:\s+(?P<out_packets>\d+)\s+packets.*?(?P<out_bytes>\d+)\s+bytes
`
	raw := `GigabitEthernet0/0/0 current state : UP
Line protocol current state : UP
Description: to-PE1-GE0/0/0
The Maximum Transmit Unit is 1500
Internet Address is 10.0.0.1/30
Input:  123456789 packets, 98765432100 bytes
Output: 987654321 packets, 12345678900 bytes
Last 300 seconds input rate 1234 bits/sec, 5 packets/sec
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if result.Mode != "record" {
		t.Errorf("expected mode=record, got %q", result.Mode)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}

	row := result.Rows[0]
	assertField(t, row, "interface", "GigabitEthernet0/0/0")
	assertField(t, row, "status", "UP")
	assertField(t, row, "protocol", "UP")
	assertField(t, row, "description", "to-PE1-GE0/0/0")
	assertField(t, row, "in_packets", "123456789")
	assertField(t, row, "in_bytes", "98765432100")
	assertField(t, row, "out_packets", "987654321")
	assertField(t, row, "out_bytes", "12345678900")
}

// ── Test: REPLACE before extraction ──────────────────────────────────────

func TestExecPipeline_ReplaceBeforeRegex(t *testing.T) {
	dsl := `
REPLACE     \([^)]+\)  ""
REGEX       ^Interface:\s*(?P<interface>\S+)
REGEX       ^\s*Status:\s*(?P<status>\S+)
`
	raw := `Interface: GigabitEthernet0/0/0(10GE)
  Status: up(up)
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	assertField(t, result.Rows[0], "interface", "GigabitEthernet0/0/0")
	assertField(t, result.Rows[0], "status", "up")
}

// ── Test: REPLACE with empty replacement (quoted empty string) ───────────

func TestExecPipeline_ReplaceEmpty(t *testing.T) {
	dsl := `
REPLACE  \([^)]+\) ""
REGEX    ^Interface:\s*(?P<name>\S+)
`
	raw := `Interface: GE0/0/0(10GE)
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	assertField(t, result.Rows[0], "name", "GE0/0/0")
}

// ── Test: STOP_AT ────────────────────────────────────────────────────────

func TestExecPipeline_StopAt(t *testing.T) {
	dsl := `
SKIP_UNTIL  ^Interface\s+PHY
STOP_AT     ^---
SPLIT       $interface $phy $protocol
`
	raw := `Interface         PHY      Protocol
GE0/0/0           up       up
GE0/0/1           down     down
--- End of table ---
GE0/0/2           up       up
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows (stopped before GE0/0/2), got %d", len(result.Rows))
	}
	assertField(t, result.Rows[0], "interface", "GE0/0/0")
	assertField(t, result.Rows[1], "interface", "GE0/0/1")
}

// ── Test: FILTER ─────────────────────────────────────────────────────────

func TestExecPipeline_Filter(t *testing.T) {
	dsl := `
SKIP_UNTIL  ^Interface\s+PHY
FILTER      \bup\b
SPLIT       $interface $phy $protocol
`
	raw := `Interface         PHY      Protocol
GE0/0/0           up       up
GE0/0/1           down     down
GE0/0/2           up       down
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	// Only lines containing "up" are kept
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows (only 'up' lines), got %d", len(result.Rows))
	}
	assertField(t, result.Rows[0], "interface", "GE0/0/0")
	assertField(t, result.Rows[1], "interface", "GE0/0/2")
}

// ── Test: REJECT ─────────────────────────────────────────────────────────

func TestExecPipeline_Reject(t *testing.T) {
	dsl := `
SKIP_UNTIL  ^Interface\s+PHY
REJECT      \bdown\b
SPLIT       $interface $phy $protocol
`
	raw := `Interface         PHY      Protocol
GE0/0/0           up       up
GE0/0/1           down     down
GE0/0/2           up       up
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows (rejected 'down' line), got %d", len(result.Rows))
	}
	assertField(t, result.Rows[0], "interface", "GE0/0/0")
	assertField(t, result.Rows[1], "interface", "GE0/0/2")
}

// ── Test: SET with concatenation ─────────────────────────────────────────

func TestExecPipeline_SetConcat(t *testing.T) {
	dsl := `
SKIP_UNTIL  ^Slot\s+Port
SPLIT       $slot $port $status
SET         $full_port $slot "/" $port
`
	raw := `Slot  Port  Status
0     1     up
0     2     down
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}
	assertField(t, result.Rows[0], "full_port", "0/1")
	assertField(t, result.Rows[1], "full_port", "0/2")
}

// ── Test: SET with ternary ───────────────────────────────────────────────

func TestExecPipeline_SetTernary(t *testing.T) {
	dsl := `
SKIP_UNTIL  ^Interface\s+Status
SPLIT       $interface $status
SET         $icon $status == up ? ✓ : ✗
`
	raw := `Interface  Status
GE0/0/0    up
GE0/0/1    down
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}
	assertField(t, result.Rows[0], "icon", "✓")
	assertField(t, result.Rows[1], "icon", "✗")
}

// ── Test: SET with REGEX_MATCH ternary ───────────────────────────────────

func TestExecPipeline_SetRegexMatch(t *testing.T) {
	dsl := `
SKIP_UNTIL  ^Interface\s+Physical\s+Protocol
SKIP_BLANK
SPLIT       $interface $physical $protocol $ip_address $description
SET $physical_clean ($physical == "*down" ? "down" : $physical)
SET $admin_down ($physical == "*down" ? "yes" : "no")
SET $protocol_clean (REGEX_MATCH($protocol, "^up\\(s\\)$") ? "up" : $protocol)
SET $spoofing (REGEX_MATCH($protocol, "\\(s\\)") ? "yes" : "no")
`
	raw := `*down: administratively down
(s): spoofing
Interface                Physical  Protocol  IP Address      Description
FGE2/0/1                 up        up        10.48.58.244    SH-YQ-050
FGE2/0/2                 *down     down      10.48.58.246    SH-YQ-051
FGE2/0/3                 up        up(s)     10.48.58.248    SH-YQ-052
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result.Rows))
	}

	// Row 0: FGE2/0/1 — up/up → physical_clean=up, admin_down=no, protocol_clean=up, spoofing=no
	assertField(t, result.Rows[0], "physical_clean", "up")
	assertField(t, result.Rows[0], "admin_down", "no")
	assertField(t, result.Rows[0], "protocol_clean", "up")
	assertField(t, result.Rows[0], "spoofing", "no")

	// Row 1: FGE2/0/2 — *down/down → physical_clean=down, admin_down=yes, protocol_clean=down, spoofing=no
	assertField(t, result.Rows[1], "physical_clean", "down")
	assertField(t, result.Rows[1], "admin_down", "yes")
	assertField(t, result.Rows[1], "protocol_clean", "down")
	assertField(t, result.Rows[1], "spoofing", "no")

	// Row 2: FGE2/0/3 — up/up(s) → physical_clean=up, admin_down=no, protocol_clean=up, spoofing=yes
	assertField(t, result.Rows[2], "physical_clean", "up")
	assertField(t, result.Rows[2], "admin_down", "no")
	assertField(t, result.Rows[2], "protocol_clean", "up")
	assertField(t, result.Rows[2], "spoofing", "yes")
}

// ── Test: SKIP_LINES after SKIP_UNTIL ────────────────────────────────────

func TestExecPipeline_SkipLines(t *testing.T) {
	dsl := `
SKIP_UNTIL  ^Interface\s+Status
SKIP_LINES  1
SPLIT       $interface $status
`
	raw := `Interface  Status
==========  ======
GE0/0/0    up
GE0/0/1    down
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows (skipped separator line), got %d", len(result.Rows))
	}
	assertField(t, result.Rows[0], "interface", "GE0/0/0")
}

// ── Test: Empty DSL ──────────────────────────────────────────────────────

func TestExecPipeline_EmptyDSL(t *testing.T) {
	_, err := ExecPipeline("", "some input")
	if err == nil {
		t.Fatal("expected error for empty DSL")
	}
}

// ── Test: Comments and blank lines in DSL ────────────────────────────────

func TestExecPipeline_CommentsIgnored(t *testing.T) {
	dsl := `
# This is a comment
SKIP_UNTIL  ^Header

# Another comment
SPLIT       $a $b

# Trailing comment
`
	raw := `Header  Col
foo     bar
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	assertField(t, result.Rows[0], "a", "foo")
	assertField(t, result.Rows[0], "b", "bar")
}

// ── Test: No header match returns empty ──────────────────────────────────

func TestExecPipeline_NoHeaderMatch(t *testing.T) {
	dsl := `
SKIP_UNTIL  ^NONEXISTENT_HEADER
SPLIT       $a $b
`
	raw := `Some random text
that has no matching header
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Errorf("expected 0 rows when header not found, got %d", len(result.Rows))
	}
}

// ── Test: REPLACE in table mode ──────────────────────────────────────────

func TestExecPipeline_ReplaceInTableMode(t *testing.T) {
	dsl := `
SKIP_UNTIL ^Name\s+Status
REPLACE    \(.*?\)  ""
SPLIT      $name $status
`
	raw := `Name          Status
GE0/0/0(10G)  up
GE0/0/1(1G)   down
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}
	assertField(t, result.Rows[0], "name", "GE0/0/0")
	assertField(t, result.Rows[0], "status", "up")
	assertField(t, result.Rows[1], "name", "GE0/0/1")
	assertField(t, result.Rows[1], "status", "down")
}

// ── Test: JSON serialization ─────────────────────────────────────────────

func TestPipelineResult_JSON(t *testing.T) {
	result := PipelineResult{
		Rows:    []map[string]string{{"a": "1", "b": "2"}},
		Columns: []string{"a", "b"},
		Mode:    "table",
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PipelineResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Mode != "table" {
		t.Errorf("expected mode=table after JSON roundtrip, got %q", decoded.Mode)
	}
	if len(decoded.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(decoded.Rows))
	}
}

// ── Test: SECTION — multi-section join (LACP Local + Remote) ─────────────

func TestExecPipeline_SectionLACP(t *testing.T) {
	dsl := `
# Section 1: Local ports
SKIP_UNTIL ^Port\s+Status\s+Priority
SKIP_LINES 1
SKIP_BLANK
STOP_AT ^Remote:
REJECT ^-+$
REPLACE \{|\} ""
SPLIT $local_port $local_status $local_priority $local_oper_key $local_flag

SECTION

# Section 2: Remote ports
SKIP_UNTIL ^Actor\s+Partner\s+Priority
SKIP_LINES 1
SKIP_BLANK
STOP_AT ^<
REJECT ^-+$
REPLACE \{|\} ""
SPLIT $remote_port $remote_partner $remote_priority $remote_oper_key $remote_sys1 $remote_sys2 $remote_flag
SET $remote_systemid $remote_sys1 " " $remote_sys2
`

	raw := `Loadsharing Type: Shar -- Loadsharing, NonS -- Non-Loadsharing
Port Status: S -- Selected, U -- Unselected, I -- Individual
Flags:  A -- LACP_Activity, B -- LACP_Timeout, C -- Aggregation,
D -- Synchronization, E -- Collecting, F -- Distributing,
G -- Defaulted, H -- Expired

Aggregate Interface: Bridge-Aggregation1
Aggregation Mode: Dynamic
Loadsharing Type: Shar
System ID: 0x8000, 3c8c-4004-d3d0
Local:
Port             Status  Priority Oper-Key  Flag
--------------------------------------------------------------------------------
XGE2/0/35:1      S       32768    1         {ACDEF}
XGE3/0/35:1      S       32768    1         {ACDEF}
Remote:
Actor            Partner Priority Oper-Key  SystemID               Flag
--------------------------------------------------------------------------------
XGE2/0/35:1      5       32768    4         0x8000, 741f-4a2e-0aa0 {ACDEF}
XGE3/0/35:1      6       32768    4         0x8000, 741f-4a2e-0aa0 {ACDEF}
<SH-YQ-0502-E16-H12516AF-LC-01>
`

	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if result.Mode != "table" {
		t.Errorf("expected mode=table, got %q", result.Mode)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d (rows: %v)", len(result.Rows), result.Rows)
	}

	// Row 0: XGE2/0/35:1 — local + remote
	assertField(t, result.Rows[0], "local_port", "XGE2/0/35:1")
	assertField(t, result.Rows[0], "local_status", "S")
	assertField(t, result.Rows[0], "local_priority", "32768")
	assertField(t, result.Rows[0], "local_oper_key", "1")
	assertField(t, result.Rows[0], "local_flag", "ACDEF")
	assertField(t, result.Rows[0], "remote_port", "XGE2/0/35:1")
	assertField(t, result.Rows[0], "remote_partner", "5")
	assertField(t, result.Rows[0], "remote_oper_key", "4")
	assertField(t, result.Rows[0], "remote_systemid", "0x8000, 741f-4a2e-0aa0")
	assertField(t, result.Rows[0], "remote_flag", "ACDEF")

	// Row 1: XGE3/0/35:1
	assertField(t, result.Rows[1], "local_port", "XGE3/0/35:1")
	assertField(t, result.Rows[1], "local_status", "S")
	assertField(t, result.Rows[1], "remote_port", "XGE3/0/35:1")
	assertField(t, result.Rows[1], "remote_partner", "6")
	assertField(t, result.Rows[1], "remote_systemid", "0x8000, 741f-4a2e-0aa0")

	// Check columns include both sections
	colSet := map[string]bool{}
	for _, c := range result.Columns {
		colSet[c] = true
	}
	for _, expected := range []string{"local_port", "local_status", "local_flag", "remote_port", "remote_partner", "remote_flag", "remote_systemid"} {
		if !colSet[expected] {
			t.Errorf("expected column %q not found in result columns: %v", expected, result.Columns)
		}
	}
}

// ── Test: SECTION — sections with different row counts ───────────────────

func TestExecPipeline_SectionUnevenRows(t *testing.T) {
	dsl := `
SKIP_UNTIL ^Name
SPLIT $name $value
STOP_AT ^---

SECTION

SKIP_UNTIL ^Extra
SPLIT $extra
`

	raw := `Name   Value
alpha  1
beta   2
gamma  3
--- end ---
Extra
x
y
`

	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 rows (max of sections), got %d", len(result.Rows))
	}
	assertField(t, result.Rows[0], "name", "alpha")
	assertField(t, result.Rows[0], "extra", "x")
	assertField(t, result.Rows[1], "name", "beta")
	assertField(t, result.Rows[1], "extra", "y")
	assertField(t, result.Rows[2], "name", "gamma")
	// Row 2 has no extra (section 2 only had 2 rows)
	if result.Rows[2]["extra"] != "" {
		t.Errorf("expected empty extra for row 2, got %q", result.Rows[2]["extra"])
	}
}

// ── Test: REPEAT_FOR — H3C display acl all (multiple ACLs with rules) ────

func TestExecPipeline_RepeatFor_ACL(t *testing.T) {
	dsl := `
# Split by ACL headers, capture parent fields
REPEAT_FOR ^(?P<acl_type>Basic|Advanced|Layer 2) IPv4 ACL (?P<acl_number>\d+) named (?P<acl_name>[^,]+),
# Only keep rule lines within each block
FILTER ^rule\s+\d+
# Extract rule fields
REGEX ^rule\s+(?P<rule_id>\d+)\s+(?P<action>permit|deny)(?:\s+(?:vpn-instance\s+(?P<vpn>\S+)))?(?:\s+source\s+(?P<source_ip>\S+)\s+(?P<source_wildcard>\S+))?(?:\s+\((?P<hit_count>\d+) times matched\))?
`
	raw := `Basic IPv4 ACL 2000 named SSH-Filter, 15 rules,
SSH-Filter
ACL's step is 5, start ID is 0
rule 0 permit source 172.16.0.192 0.0.0.63
rule 5 permit source 172.17.73.0 0.0.0.31
rule 10 permit source 172.18.62.0 0.0.0.31
rule 35 permit vpn-instance mgt_vrf source 172.16.0.192 0.0.0.63 (3939 times matched)

Basic IPv4 ACL 2001 named SNMP-Filter, 13 rules,
SNMP-Filter
ACL's step is 5, start ID is 0
rule 0 permit source 172.16.0.192 0.0.0.63
rule 25 permit vpn-instance mgt_vrf source 172.16.0.192 0.0.0.63 (707154 times matched)
rule 50 permit vpn-instance mgt_vrf source 10.185.218.231 0 (27429048 times matched)

<GZ-HXY-G160303-E03-H12504AF-DR-30101>
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if result.Mode != "table" {
		t.Errorf("expected mode=table, got %q", result.Mode)
	}

	// ACL 2000: 4 rules + ACL 2001: 3 rules = 7 total rows
	if len(result.Rows) != 7 {
		t.Fatalf("expected 7 rows, got %d; rows: %v", len(result.Rows), result.Rows)
	}

	// First 4 rows should belong to ACL 2000 (SSH-Filter)
	assertField(t, result.Rows[0], "acl_type", "Basic")
	assertField(t, result.Rows[0], "acl_number", "2000")
	assertField(t, result.Rows[0], "acl_name", "SSH-Filter")
	assertField(t, result.Rows[0], "rule_id", "0")
	assertField(t, result.Rows[0], "action", "permit")
	assertField(t, result.Rows[0], "source_ip", "172.16.0.192")
	assertField(t, result.Rows[0], "source_wildcard", "0.0.0.63")

	// Row with VPN instance
	assertField(t, result.Rows[3], "acl_number", "2000")
	assertField(t, result.Rows[3], "rule_id", "35")
	assertField(t, result.Rows[3], "vpn", "mgt_vrf")
	assertField(t, result.Rows[3], "hit_count", "3939")

	// Rows 4-6 belong to ACL 2001 (SNMP-Filter)
	assertField(t, result.Rows[4], "acl_type", "Basic")
	assertField(t, result.Rows[4], "acl_number", "2001")
	assertField(t, result.Rows[4], "acl_name", "SNMP-Filter")
	assertField(t, result.Rows[4], "rule_id", "0")

	assertField(t, result.Rows[6], "acl_number", "2001")
	assertField(t, result.Rows[6], "rule_id", "50")
	assertField(t, result.Rows[6], "hit_count", "27429048")

	// Check parent columns are included
	colSet := map[string]bool{}
	for _, c := range result.Columns {
		colSet[c] = true
	}
	for _, expected := range []string{"acl_type", "acl_number", "acl_name", "rule_id", "action", "source_ip"} {
		if !colSet[expected] {
			t.Errorf("expected column %q not found in result columns: %v", expected, result.Columns)
		}
	}
}

// ── Test: REPEAT_FOR — no matching headers returns empty ─────────────────

func TestExecPipeline_RepeatFor_NoMatch(t *testing.T) {
	dsl := `
REPEAT_FOR ^(?P<group>GROUP \d+)
SPLIT $name $value
`
	raw := `some random text
without any matching headers
`
	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Errorf("expected 0 rows for no header match, got %d", len(result.Rows))
	}
}

// ── Test: Nested REPEAT_FOR — route-policy with multiple permit/deny nodes ──

func TestExecPipeline_NestedRepeatFor_RoutePolicy(t *testing.T) {
	dsl := `
# Level 1: split by route-policy name
STOP_AT ^<
REPEAT_FOR ^Route-policy:\s+(?P<route_policy_name>.+)$
# Level 2: split by permit/deny lines within each route-policy block
REPEAT_FOR ^(?P<action>permit|deny)\s*:\s*(?P<seq>\d+)\s*\(matched counts:\s*(?P<matched_counts>\d+)\)
REGEX ^if-match\s+(?P<match_clause>.+)$
REGEX ^apply\s+(?P<apply_clause>.+)$
`
	raw := `Route-policy: 1:4
permit : 10 (matched counts: 736455)
Apply clauses:
apply community 1:4
Route-policy: From-DR
permit : 5 (matched counts: 5219730)
Match clauses:
if-match community-filter 11
Route-policy: To-LC
permit : 4 (matched counts: 738982)
Match clauses:
if-match community-filter To-LC-no-advertise
Apply clauses:
apply community no-advertise additive
permit : 5 (matched counts: 22837810)
Match clauses:
if-match community-filter 20
Route-policy: From-SDN-Controller
permit : 5 (matched counts: 0)
Match clauses:
if-match community-filter SDN-Controller
Apply clauses:
apply community no-advertise additive
apply preferred-value 32768
<GZ-HXY-G160304-B02-HW12816-CUF-13>`

	result, err := ExecPipeline(dsl, raw)
	if err != nil {
		t.Fatalf("ExecPipeline error: %v", err)
	}
	if result.Mode != "table" {
		t.Errorf("expected mode=table, got %q", result.Mode)
	}

	// Expected rows:
	// 1:4       → permit:10 (apply=community 1:4)
	// From-DR   → permit:5  (match=community-filter 11)
	// To-LC     → permit:4  (match=..., apply=...) + permit:5 (match=community-filter 20)
	// From-SDN  → permit:5  (match=..., apply=...)
	// Total: 5 rows
	if len(result.Rows) != 5 {
		t.Fatalf("expected 5 rows, got %d; rows: %v", len(result.Rows), result.Rows)
	}

	// Row 0: 1:4 / permit / 10
	assertField(t, result.Rows[0], "route_policy_name", "1:4")
	assertField(t, result.Rows[0], "action", "permit")
	assertField(t, result.Rows[0], "seq", "10")
	assertField(t, result.Rows[0], "matched_counts", "736455")
	assertField(t, result.Rows[0], "apply_clause", "community 1:4")

	// Row 1: From-DR / permit / 5
	assertField(t, result.Rows[1], "route_policy_name", "From-DR")
	assertField(t, result.Rows[1], "action", "permit")
	assertField(t, result.Rows[1], "seq", "5")
	assertField(t, result.Rows[1], "match_clause", "community-filter 11")

	// Row 2: To-LC / permit / 4
	assertField(t, result.Rows[2], "route_policy_name", "To-LC")
	assertField(t, result.Rows[2], "action", "permit")
	assertField(t, result.Rows[2], "seq", "4")
	assertField(t, result.Rows[2], "matched_counts", "738982")
	assertField(t, result.Rows[2], "match_clause", "community-filter To-LC-no-advertise")
	assertField(t, result.Rows[2], "apply_clause", "community no-advertise additive")

	// Row 3: To-LC / permit / 5 (second node)
	assertField(t, result.Rows[3], "route_policy_name", "To-LC")
	assertField(t, result.Rows[3], "action", "permit")
	assertField(t, result.Rows[3], "seq", "5")
	assertField(t, result.Rows[3], "matched_counts", "22837810")
	assertField(t, result.Rows[3], "match_clause", "community-filter 20")

	// Row 4: From-SDN-Controller / permit / 5
	assertField(t, result.Rows[4], "route_policy_name", "From-SDN-Controller")
	assertField(t, result.Rows[4], "action", "permit")
	assertField(t, result.Rows[4], "match_clause", "community-filter SDN-Controller")
	// This node has 2 apply clauses but record mode takes first match only
	assertField(t, result.Rows[4], "apply_clause", "community no-advertise additive")

	// Check column order: parent cols first, then child cols
	colSet := map[string]bool{}
	for _, c := range result.Columns {
		colSet[c] = true
	}
	for _, expected := range []string{"route_policy_name", "action", "seq", "matched_counts", "match_clause", "apply_clause"} {
		if !colSet[expected] {
			t.Errorf("expected column %q not found in result columns: %v", expected, result.Columns)
		}
	}
}

// ── Helper ───────────────────────────────────────────────────────────────

func assertField(t *testing.T, row map[string]string, key, expected string) {
	t.Helper()
	got, ok := row[key]
	if !ok {
		t.Errorf("field %q not found in row (keys: %v)", key, mapKeys(row))
		return
	}
	if got != expected {
		t.Errorf("field %q: expected %q, got %q", key, expected, got)
	}
}

func mapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
