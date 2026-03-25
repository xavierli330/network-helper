package parser

import (
	"sort"

	"github.com/xavierli/nethelper/internal/model"
)

// FieldRegistry is an in-memory index of field definitions,
// keyed by vendor → CommandType.
type FieldRegistry struct {
	index map[string]map[model.CommandType][]FieldDef
	order []string // vendor insertion order
	reg   *Registry // kept for ClassifyCommand lookups in the Studio API
}

// BuildFieldRegistry iterates every registered VendorParser, calls
// SupportedCmdTypes() to get the full list, then collects FieldDef
// slices via FieldSchema().
func BuildFieldRegistry(reg *Registry) *FieldRegistry {
	fr := &FieldRegistry{
		index: make(map[string]map[model.CommandType][]FieldDef),
		reg:   reg,
	}
	for _, p := range reg.Parsers() {
		vendor := p.Vendor()
		if _, exists := fr.index[vendor]; !exists {
			fr.order = append(fr.order, vendor)
		}
		fr.index[vendor] = make(map[model.CommandType][]FieldDef)
		for _, ct := range p.SupportedCmdTypes() {
			fr.index[vendor][ct] = p.FieldSchema(ct) // nil schema is valid
		}
	}
	return fr
}

// Fields returns the FieldDef list for the given vendor + CommandType.
// Returns nil if the vendor or CommandType is not registered.
func (r *FieldRegistry) Fields(vendor string, cmdType model.CommandType) []FieldDef {
	if m, ok := r.index[vendor]; ok {
		return m[cmdType]
	}
	return nil
}

// Vendors returns all vendor names in registration order.
func (r *FieldRegistry) Vendors() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// CmdTypes returns all CommandType values registered for the given vendor,
// sorted alphabetically for deterministic output.
func (r *FieldRegistry) CmdTypes(vendor string) []model.CommandType {
	m, ok := r.index[vendor]
	if !ok {
		return nil
	}
	types := make([]model.CommandType, 0, len(m))
	for ct := range m {
		types = append(types, ct)
	}
	sort.Slice(types, func(i, j int) bool {
		return string(types[i]) < string(types[j])
	})
	return types
}

// ClassifyCommand resolves a raw command string to a CommandType for the given vendor.
// Returns model.CmdUnknown if the vendor is unknown.
func (r *FieldRegistry) ClassifyCommand(vendor, rawCmd string) model.CommandType {
	p, ok := r.reg.Get(vendor)
	if !ok {
		return model.CmdUnknown
	}
	return p.ClassifyCommand(rawCmd)
}

// Reg returns the underlying parser Registry, needed for live command classification
// in the Studio parser tester.
func (r *FieldRegistry) Reg() *Registry {
	return r.reg
}
