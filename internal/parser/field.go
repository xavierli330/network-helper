package parser

import "github.com/xavierli/nethelper/internal/model"

// FieldType and FieldDef are defined in internal/model and re-exported here
// as type aliases so that existing parser-package references compile unchanged.
type FieldType = model.FieldType
type FieldDef = model.FieldDef

// Re-export the FieldType constants so callers using parser.FieldTypeString etc.
// continue to work.
const (
	FieldTypeString = model.FieldTypeString
	FieldTypeInt    = model.FieldTypeInt
	FieldTypeFloat  = model.FieldTypeFloat
	FieldTypeBool   = model.FieldTypeBool
)
