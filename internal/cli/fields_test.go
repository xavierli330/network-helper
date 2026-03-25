package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/parser"
)

func TestRuleFieldsCmd_NoArgs(t *testing.T) {
	reg := parser.NewRegistry()
	reg.Register(&stubVendorParser{vendor: "testvendor"})
	fr := parser.BuildFieldRegistry(reg)

	var buf bytes.Buffer
	cmd := newRuleFieldsCmd(fr, reg)
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "testvendor") {
		t.Errorf("expected testvendor in output, got: %s", out)
	}
}

func TestRuleFieldsCmd_VendorOnly(t *testing.T) {
	reg := parser.NewRegistry()
	reg.Register(&stubVendorParser{
		vendor: "testvendor",
		types:  []model.CommandType{model.CmdInterface},
	})
	fr := parser.BuildFieldRegistry(reg)

	var buf bytes.Buffer
	cmd := newRuleFieldsCmd(fr, reg)
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"testvendor"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "interface") {
		t.Errorf("expected 'interface' in output, got: %s", out)
	}
}

func TestRuleFieldsCmd_VendorAndCommand(t *testing.T) {
	defs := []parser.FieldDef{
		{Name: "name", Type: parser.FieldTypeString, Description: "接口名称", Example: "GE0"},
	}
	reg := parser.NewRegistry()
	reg.Register(&stubVendorParser{
		vendor: "testvendor",
		types:  []model.CommandType{model.CmdInterface},
		schema: map[model.CommandType][]parser.FieldDef{
			model.CmdInterface: defs,
		},
	})
	fr := parser.BuildFieldRegistry(reg)

	var buf bytes.Buffer
	cmd := newRuleFieldsCmd(fr, reg)
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"testvendor", "display interface brief"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "name") {
		t.Errorf("expected 'name' field in output, got: %s", out)
	}
}

// stubVendorParser satisfies parser.VendorParser for tests.
type stubVendorParser struct {
	vendor string
	types  []model.CommandType
	schema map[model.CommandType][]parser.FieldDef
}

func (s *stubVendorParser) Vendor() string                     { return s.vendor }
func (s *stubVendorParser) DetectPrompt(string) (string, bool) { return "", false }
func (s *stubVendorParser) ClassifyCommand(cmd string) model.CommandType {
	if strings.Contains(cmd, "interface") {
		return model.CmdInterface
	}
	return model.CmdUnknown
}
func (s *stubVendorParser) ParseOutput(model.CommandType, string) (model.ParseResult, error) {
	return model.ParseResult{}, nil
}
func (s *stubVendorParser) SupportedCmdTypes() []model.CommandType { return s.types }
func (s *stubVendorParser) FieldSchema(ct model.CommandType) []parser.FieldDef {
	if s.schema != nil {
		return s.schema[ct]
	}
	return nil
}
