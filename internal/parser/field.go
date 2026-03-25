package parser

// FieldType is the set of legal field value kinds.
type FieldType string

const (
	FieldTypeString FieldType = "string"
	FieldTypeInt    FieldType = "int"
	FieldTypeFloat  FieldType = "float"
	FieldTypeBool   FieldType = "bool"
)

// FieldDef describes one output field produced by a parsed command.
type FieldDef struct {
	Name        string    // snake_case identifier, e.g. "phy_status"
	Type        FieldType // one of the FieldType constants
	Description string    // human-readable description
	Example     string    // representative value, e.g. "up"
	Derived     bool      // true if computed from other fields
	DerivedFrom []string  // source field names for derived fields
}
