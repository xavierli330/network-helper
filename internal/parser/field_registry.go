package parser

import "github.com/xavierli/nethelper/internal/model"

// FieldRegistry is an in-memory index of field definitions,
// keyed by vendor → CommandType.
type FieldRegistry struct {
	index map[string]map[model.CommandType][]FieldDef
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
		fr.index[vendor] = make(map[model.CommandType][]FieldDef)
		for _, ct := range p.SupportedCmdTypes() {
			if defs := p.FieldSchema(ct); len(defs) > 0 {
				fr.index[vendor][ct] = defs
			}
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
	vendors := make([]string, 0, len(r.index))
	for v := range r.index {
		vendors = append(vendors, v)
	}
	return vendors
}

// CmdTypes returns all CommandType values registered for the given vendor.
func (r *FieldRegistry) CmdTypes(vendor string) []model.CommandType {
	m, ok := r.index[vendor]
	if !ok {
		return nil
	}
	types := make([]model.CommandType, 0, len(m))
	for ct := range m {
		types = append(types, ct)
	}
	return types
}

// ClassifyCommand resolves a raw command string to a CommandType for the given vendor.
// Returns empty string ("") if the vendor is unknown.
func (r *FieldRegistry) ClassifyCommand(vendor, rawCmd string) model.CommandType {
	p, ok := r.reg.Get(vendor)
	if !ok {
		return ""
	}
	return p.ClassifyCommand(rawCmd)
}
