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
