package parser

import (
	"time"

	"github.com/xavierli/nethelper/internal/model"
)

type VendorParser interface {
	Vendor() string
	DetectPrompt(line string) (hostname string, ok bool)
	ClassifyCommand(cmd string) model.CommandType
	ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error)
	// SupportedCmdTypes returns all CommandType values this parser handles,
	// including any dynamically registered generated types.
	SupportedCmdTypes() []model.CommandType
	// FieldSchema returns the field definitions for the given CommandType.
	// Returns nil (not an error) for unknown types.
	FieldSchema(cmdType model.CommandType) []FieldDef
}

type CommandBlock struct {
	Hostname   string
	Vendor     string
	Command    string
	Output     string
	CmdType    model.CommandType
	CapturedAt time.Time // extracted from log timestamp prefix; zero if absent
}

type Registry struct {
	parsers map[string]VendorParser
	order   []string // preserve insertion order for deterministic iteration
}

func NewRegistry() *Registry {
	return &Registry{parsers: make(map[string]VendorParser)}
}

func (r *Registry) Register(p VendorParser) {
	if _, exists := r.parsers[p.Vendor()]; !exists {
		r.order = append(r.order, p.Vendor())
	}
	r.parsers[p.Vendor()] = p
}

func (r *Registry) Get(vendor string) (VendorParser, bool) {
	p, ok := r.parsers[vendor]
	return p, ok
}

func (r *Registry) Parsers() []VendorParser {
	result := make([]VendorParser, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.parsers[name])
	}
	return result
}
