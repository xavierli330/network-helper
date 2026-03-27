// internal/parser/engine/table_test.go
package engine_test

import (
	"testing"
	"github.com/xavierli/nethelper/internal/parser/engine"
)

var briefOutput = `Interface         PHY      Protocol  InUti  OutUti  inErrors outErrors
GigabitEthernet0/0/0  up       up        0.01%  0.01%       0        0
GigabitEthernet0/0/1  down     down         --     --       0        0
Eth-Trunk1            up       up        1.23%  0.45%       0        0
`

func TestParseTableBasic(t *testing.T) {
	schema := engine.TableSchema{
		HeaderPattern: `Interface\s+PHY`,
		SkipLines:     0,
		Columns: []engine.ColumnDef{
			{Name: "interface", Index: 0, Type: "string"},
			{Name: "phy_status", Index: 1, Type: "string"},
			{Name: "proto_status", Index: 2, Type: "string"},
		},
	}

	result, err := engine.ParseTable(schema, briefOutput)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(result.Rows))
	}
	if result.Rows[0]["interface"] != "GigabitEthernet0/0/0" {
		t.Errorf("unexpected interface: %q", result.Rows[0]["interface"])
	}
	if result.Rows[1]["phy_status"] != "down" {
		t.Errorf("unexpected phy_status: %q", result.Rows[1]["phy_status"])
	}
}

func TestParseTableNoHeaderMatch(t *testing.T) {
	schema := engine.TableSchema{
		HeaderPattern: `NONEXISTENT_HEADER`,
		Columns:       []engine.ColumnDef{{Name: "col", Index: 0, Type: "string"}},
	}
	result, err := engine.ParseTable(schema, briefOutput)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 0 {
		t.Errorf("want 0 rows when header not found, got %d", len(result.Rows))
	}
}

// ── Auto-columns tests ──────────────────────────────────────────────────

// Test auto-detection of columns from header when schema.Columns is empty (nil).
func TestParseTable_AutoColumns_Nil(t *testing.T) {
	schema := engine.TableSchema{
		HeaderPattern: `Interface\s+PHY`,
		SkipLines:     0,
		// Columns intentionally nil — triggers auto-detect
	}

	result, err := engine.ParseTable(schema, briefOutput)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(result.Rows))
	}
	// Auto-detected columns should be present
	if len(result.AutoColumns) == 0 {
		t.Fatal("expected AutoColumns to be populated")
	}
	// Header is: Interface PHY Protocol InUti OutUti inErrors outErrors → 7 columns
	if len(result.AutoColumns) != 7 {
		t.Fatalf("want 7 auto columns, got %d", len(result.AutoColumns))
	}
	// Check normalised names
	wantNames := []string{"interface", "phy", "protocol", "inuti", "oututi", "inerrors", "outerrors"}
	for i, want := range wantNames {
		if result.AutoColumns[i].Name != want {
			t.Errorf("auto col %d: name=%q, want %q", i, result.AutoColumns[i].Name, want)
		}
		if result.AutoColumns[i].Index != i {
			t.Errorf("auto col %d: index=%d, want %d", i, result.AutoColumns[i].Index, i)
		}
	}
	// Verify data was parsed correctly
	if result.Rows[0]["interface"] != "GigabitEthernet0/0/0" {
		t.Errorf("row 0 interface = %q", result.Rows[0]["interface"])
	}
	if result.Rows[0]["phy"] != "up" {
		t.Errorf("row 0 phy = %q", result.Rows[0]["phy"])
	}
	if result.Rows[0]["protocol"] != "up" {
		t.Errorf("row 0 protocol = %q", result.Rows[0]["protocol"])
	}
}

// Test auto-detection with empty slice (not nil).
func TestParseTable_AutoColumns_EmptySlice(t *testing.T) {
	schema := engine.TableSchema{
		HeaderPattern: `Interface\s+PHY`,
		SkipLines:     0,
		Columns:       []engine.ColumnDef{}, // empty slice — also triggers auto-detect
	}

	result, err := engine.ParseTable(schema, briefOutput)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AutoColumns) == 0 {
		t.Fatal("expected AutoColumns with empty Columns slice")
	}
	if len(result.Rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(result.Rows))
	}
}

// Test that explicit columns do NOT produce AutoColumns.
func TestParseTable_ExplicitColumns_NoAutoColumns(t *testing.T) {
	schema := engine.TableSchema{
		HeaderPattern: `Interface\s+PHY`,
		Columns: []engine.ColumnDef{
			{Name: "iface", Index: 0, Type: "string"},
		},
	}
	result, err := engine.ParseTable(schema, briefOutput)
	if err != nil {
		t.Fatal(err)
	}
	if result.AutoColumns != nil {
		t.Error("AutoColumns should be nil when explicit Columns are provided")
	}
}

// Simulate `show ip interface brief` — the exact use case from the user.
var showIPIntBrief = `*down: administratively down
(s): spoofing
Interface                     Physical  Protocol  IP Address      Description
FGE2/0/1                      up        up        10.48.58.244    SH-YQ-050...
FGE2/0/2                      up        up        10.48.58.246    SH-YQ-050...
FGE2/0/3                      up        up        10.48.58.248    SH-YQ-050...
FGE2/0/4                      up        up        10.48.58.250    SH-YQ-050...
`

func TestParseTable_AutoColumns_ShowIPIntBrief(t *testing.T) {
	schema := engine.TableSchema{
		HeaderPattern: `Interface\s+Physical\s+Protocol`,
		// No columns — auto-detect from header
	}

	result, err := engine.ParseTable(schema, showIPIntBrief)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 4 {
		t.Fatalf("want 4 rows, got %d", len(result.Rows))
	}
	// "IP Address" in header becomes two tokens: "ip" and "address" (whitespace split)
	if len(result.AutoColumns) != 6 {
		t.Fatalf("want 6 auto columns, got %d: %+v", len(result.AutoColumns), result.AutoColumns)
	}
	// Column names: interface, physical, protocol, ip, address, description
	wantCols := []string{"interface", "physical", "protocol", "ip", "address", "description"}
	for i, want := range wantCols {
		if result.AutoColumns[i].Name != want {
			t.Errorf("col %d: name=%q, want %q", i, result.AutoColumns[i].Name, want)
		}
	}
	// Verify row data — each data row also splits by whitespace
	row0 := result.Rows[0]
	if row0["interface"] != "FGE2/0/1" {
		t.Errorf("row 0 interface = %q", row0["interface"])
	}
	if row0["physical"] != "up" {
		t.Errorf("row 0 physical = %q", row0["physical"])
	}
	if row0["protocol"] != "up" {
		t.Errorf("row 0 protocol = %q", row0["protocol"])
	}
	if row0["ip"] != "10.48.58.244" {
		t.Errorf("row 0 ip = %q", row0["ip"])
	}
	if row0["address"] != "SH-YQ-050..." {
		t.Errorf("row 0 address = %q", row0["address"])
	}
}
