package parser_test

import (
	"testing"

	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/parser"
)

// stubParser implements VendorParser for testing only.
type stubParser struct {
	vendor string
	types  []model.CommandType
	schema map[model.CommandType][]parser.FieldDef
}

func (s *stubParser) Vendor() string                { return s.vendor }
func (s *stubParser) DetectPrompt(string) (string, bool) { return "", false }
func (s *stubParser) ClassifyCommand(string) model.CommandType { return model.CmdUnknown }
func (s *stubParser) ParseOutput(model.CommandType, string) (model.ParseResult, error) {
	return model.ParseResult{}, nil
}
func (s *stubParser) SupportedCmdTypes() []model.CommandType { return s.types }
func (s *stubParser) FieldSchema(ct model.CommandType) []parser.FieldDef {
	return s.schema[ct]
}

func TestBuildFieldRegistry(t *testing.T) {
	reg := parser.NewRegistry()
	reg.Register(&stubParser{
		vendor: "test",
		types:  []model.CommandType{model.CmdInterface, model.CmdNeighbor},
		schema: map[model.CommandType][]parser.FieldDef{
			model.CmdInterface: {
				{Name: "name", Type: parser.FieldTypeString, Description: "interface name", Example: "GE0/0/0"},
			},
		},
	})

	fr := parser.BuildFieldRegistry(reg)

	vendors := fr.Vendors()
	if len(vendors) != 1 || vendors[0] != "test" {
		t.Fatalf("expected [test], got %v", vendors)
	}

	types := fr.CmdTypes("test")
	if len(types) != 2 {
		t.Fatalf("expected 2 cmd types, got %d", len(types))
	}

	fields := fr.Fields("test", model.CmdInterface)
	if len(fields) != 1 || fields[0].Name != "name" {
		t.Fatalf("unexpected fields: %+v", fields)
	}

	// Unknown vendor → nil, not panic
	if fr.Fields("unknown", model.CmdInterface) != nil {
		t.Fatal("expected nil for unknown vendor")
	}

	// Known vendor, unknown cmdType → nil
	if fr.Fields("test", model.CmdRIB) != nil {
		t.Fatal("expected nil for unknown cmdType")
	}

	// Unknown vendor → nil for CmdTypes
	if fr.CmdTypes("unknown") != nil {
		t.Fatal("expected nil for unknown vendor CmdTypes")
	}

	// ClassifyCommand: unknown vendor → CmdUnknown
	if ct := fr.ClassifyCommand("unknown", "display interface"); ct != model.CmdUnknown {
		t.Fatalf("expected CmdUnknown for unknown vendor, got %q", ct)
	}

	// ClassifyCommand: known vendor delegates to parser (stubParser always returns CmdUnknown)
	if ct := fr.ClassifyCommand("test", "display interface"); ct != model.CmdUnknown {
		t.Fatalf("expected CmdUnknown from stub, got %q", ct)
	}
}
