package parser

import "github.com/xavierli/nethelper/internal/model"

type VendorParser interface {
	Vendor() string
	DetectPrompt(line string) (hostname string, ok bool)
	ClassifyCommand(cmd string) model.CommandType
	ParseOutput(cmdType model.CommandType, raw string) (model.ParseResult, error)
}

type CommandBlock struct {
	Hostname string
	Vendor   string
	Command  string
	Output   string
	CmdType  model.CommandType
}

type Registry struct {
	parsers map[string]VendorParser
}

func NewRegistry() *Registry {
	return &Registry{parsers: make(map[string]VendorParser)}
}

func (r *Registry) Register(p VendorParser) {
	r.parsers[p.Vendor()] = p
}

func (r *Registry) Get(vendor string) (VendorParser, bool) {
	p, ok := r.parsers[vendor]
	return p, ok
}

func (r *Registry) Parsers() []VendorParser {
	result := make([]VendorParser, 0, len(r.parsers))
	for _, p := range r.parsers {
		result = append(result, p)
	}
	return result
}
