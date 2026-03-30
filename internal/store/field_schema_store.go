package store

// FieldSchema represents a row in the field_schemas table.
// Stores editable metadata for fields extracted from DSL named groups (?P<name>...).
type FieldSchema struct {
	ID          int    `json:"id"`
	Vendor      string `json:"vendor"`
	CmdType     string `json:"cmd_type"`
	FieldName   string `json:"field_name"`
	FieldType   string `json:"field_type"`   // string, int, float, bool, ip, prefix
	Description string `json:"description"`
	Example     string `json:"example"`
}

// ListFieldSchemas returns field schemas filtered by vendor and/or cmd_type.
func (db *DB) ListFieldSchemas(vendor, cmdType string) ([]FieldSchema, error) {
	q := `SELECT id, vendor, cmd_type, field_name, field_type, description, example FROM field_schemas WHERE 1=1`
	var args []any
	if vendor != "" {
		q += ` AND vendor = ?`
		args = append(args, vendor)
	}
	if cmdType != "" {
		q += ` AND cmd_type = ?`
		args = append(args, cmdType)
	}
	q += ` ORDER BY vendor, cmd_type, field_name`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FieldSchema
	for rows.Next() {
		var f FieldSchema
		if err := rows.Scan(&f.ID, &f.Vendor, &f.CmdType, &f.FieldName, &f.FieldType, &f.Description, &f.Example); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// UpsertFieldSchema inserts or updates a field schema by (vendor, cmd_type, field_name).
func (db *DB) UpsertFieldSchema(f FieldSchema) (int, error) {
	res, err := db.Exec(`
		INSERT INTO field_schemas (vendor, cmd_type, field_name, field_type, description, example)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(vendor, cmd_type, field_name) DO UPDATE SET
			field_type = excluded.field_type,
			description = excluded.description,
			example = excluded.example`,
		f.Vendor, f.CmdType, f.FieldName, f.FieldType, f.Description, f.Example)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

// UpdateFieldSchema updates a field schema by ID.
func (db *DB) UpdateFieldSchema(f FieldSchema) error {
	_, err := db.Exec(`UPDATE field_schemas SET field_type=?, description=?, example=? WHERE id=?`,
		f.FieldType, f.Description, f.Example, f.ID)
	return err
}

// DeleteFieldSchema removes a field schema by ID.
func (db *DB) DeleteFieldSchema(id int) error {
	_, err := db.Exec(`DELETE FROM field_schemas WHERE id=?`, id)
	return err
}

// SyncFieldSchemasFromDSL extracts named groups (?P<name>...) from a DSL text
// and upserts field schemas for any new fields. Existing fields are not overwritten.
// Returns the number of new fields inserted.
func (db *DB) SyncFieldSchemasFromDSL(vendor, cmdType, dslText string) (int, error) {
	fields := ExtractNamedGroups(dslText)
	if len(fields) == 0 {
		return 0, nil
	}

	count := 0
	for _, fieldName := range fields {
		// Only insert if not already exists (don't overwrite user edits)
		var exists int
		err := db.QueryRow(`SELECT COUNT(*) FROM field_schemas WHERE vendor=? AND cmd_type=? AND field_name=?`,
			vendor, cmdType, fieldName).Scan(&exists)
		if err != nil {
			return count, err
		}
		if exists == 0 {
			_, err = db.Exec(`INSERT INTO field_schemas (vendor, cmd_type, field_name) VALUES (?, ?, ?)`,
				vendor, cmdType, fieldName)
			if err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

// ExtractNamedGroups extracts all (?P<name>...) group names from a regex/DSL string.
func ExtractNamedGroups(dsl string) []string {
	var names []string
	seen := make(map[string]bool)
	// Simple state-machine parser for (?P<name>...)
	for i := 0; i < len(dsl)-4; i++ {
		if dsl[i] == '(' && dsl[i+1] == '?' && dsl[i+2] == 'P' && dsl[i+3] == '<' {
			j := i + 4
			for j < len(dsl) && dsl[j] != '>' {
				j++
			}
			if j < len(dsl) {
				name := dsl[i+4 : j]
				if name != "" && !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
			}
		}
	}
	return names
}
