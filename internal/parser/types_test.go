package parser

import (
	"testing"
	"github.com/xavierli/nethelper/internal/model"
)

type mockParser struct{}
func (m *mockParser) Vendor() string { return "mock" }
func (m *mockParser) DetectPrompt(line string) (string, bool) { return "", false }
func (m *mockParser) ClassifyCommand(cmd string) model.CommandType { return model.CmdUnknown }
func (m *mockParser) ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error) {
	return model.ParseResult{}, nil
}

func TestRegisterAndGetParser(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockParser{})
	p, ok := r.Get("mock")
	if !ok { t.Fatal("expected to find mock parser") }
	if p.Vendor() != "mock" { t.Errorf("expected mock, got %s", p.Vendor()) }
	_, ok = r.Get("nonexistent")
	if ok { t.Error("should not find nonexistent parser") }
}

func TestRegistryParsers(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockParser{})
	if len(r.Parsers()) != 1 { t.Errorf("expected 1 parser, got %d", len(r.Parsers())) }
}
