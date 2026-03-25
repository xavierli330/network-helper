// internal/codegen/generator_test.go
package codegen_test

import (
	"os"
	"strings"
	"testing"

	"github.com/xavierli/nethelper/internal/codegen"
	"github.com/xavierli/nethelper/internal/store"
)

func TestCmdNameToGoIdent(t *testing.T) {
	cases := []struct{ in, want string }{
		{"display traffic-policy statistics interface", "TrafficPolicyStatisticsInterface"},
		{"show ip route", "IpRoute"},
		{"display interface", "Interface"},
	}
	for _, c := range cases {
		got := codegen.CmdNameToGoIdent(c.in)
		if got != c.want {
			t.Errorf("CmdNameToGoIdent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTargetFilePath(t *testing.T) {
	path := codegen.TargetFilePath("huawei", "display traffic-policy statistics interface")
	want := "internal/parser/huawei/traffic_policy_statistics_interface.go"
	if path != want {
		t.Errorf("TargetFilePath = %q, want %q", path, want)
	}
}

func TestGenerateParserFile_Table(t *testing.T) {
	rule := store.PendingRule{
		ID: 42, Vendor: "huawei",
		CommandPattern: "display traffic-policy statistics interface",
		OutputType:     "table",
		SchemaYAML: `header_pattern: "Interface\\s+Policy"
skip_lines: 0
columns:
  - name: interface
    index: 0
    type: string`,
		ApprovedBy: "zhangsan",
	}
	src, err := codegen.GenerateParserFile(rule)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "func ParseHuaweiTrafficPolicyStatisticsInterface") {
		t.Error("expected function name in source")
	}
	if !strings.Contains(src, "engine.ParseTable") {
		t.Error("expected engine.ParseTable call")
	}
	if !strings.Contains(src, `"github.com/xavierli/nethelper/internal/parser/engine"`) {
		t.Error("expected engine import for table rule")
	}
}

func TestGenerateParserFile_Hierarchical(t *testing.T) {
	rule := store.PendingRule{
		ID: 43, Vendor: "huawei",
		CommandPattern: "display ospf peer verbose",
		OutputType:     "hierarchical",
		GoCodeDraft:    `return model.ParseResult{RawText: raw}, nil`,
	}
	src, err := codegen.GenerateParserFile(rule)
	if err != nil {
		t.Fatal(err)
	}
	// engine import must NOT be present for non-table rules
	if strings.Contains(src, `"github.com/xavierli/nethelper/internal/parser/engine"`) {
		t.Error("engine import must not appear for hierarchical rule")
	}
}

func TestGenerateTestFile(t *testing.T) {
	rule := store.PendingRule{
		ID: 42, Vendor: "huawei",
		CommandPattern: "display traffic-policy statistics interface",
	}
	testCases := []store.RuleTestCase{{ID: 1, RuleID: 42, Input: "raw output", Expected: `{"rows":[]}`}}

	src, err := codegen.GenerateTestFile(rule, testCases)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "TestParseHuaweiTrafficPolicyStatisticsInterface") {
		t.Error("expected test function name")
	}
}

func TestPatchGeneratedFile(t *testing.T) {
	stub := `package huawei

import "github.com/xavierli/nethelper/internal/model"

func classifyGenerated(cmd string) model.CommandType {
	switch {
	// GENERATED CASES — do not edit this comment
	}
	return model.CmdUnknown
}

func parseGenerated(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	switch cmdType {
	// GENERATED PARSE CASES — do not edit this comment
	}
	return model.ParseResult{Type: cmdType, RawText: raw}, nil
}
`
	tmp, err := os.CreateTemp(t.TempDir(), "huawei_generated_*.go")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tmp.WriteString(stub); err != nil {
		t.Fatal(err)
	}
	tmp.Close()

	if err := codegen.PatchGeneratedFile(tmp.Name(), "display traffic-policy statistics interface",
		"ParseHuaweiTrafficPolicyStatisticsInterface", "huawei"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)

	if !strings.Contains(got, `strings.HasPrefix(cmd, "display traffic-policy statistics interface")`) {
		t.Error("classifyGenerated case not inserted")
	}
	if !strings.Contains(got, `generated:huawei:traffic_policy_statistics_interface`) {
		t.Error("unique cmdType string not inserted")
	}
	if !strings.Contains(got, `ParseHuaweiTrafficPolicyStatisticsInterface(raw)`) {
		t.Error("parseGenerated dispatch case not inserted")
	}
	if !strings.Contains(got, `"strings"`) {
		t.Error("strings import not added")
	}
}
