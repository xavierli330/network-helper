package store

import (
	"database/sql"
	"time"
)

// UnknownOutput represents a row in the unknown_outputs table.
type UnknownOutput struct {
	ID              int
	DeviceID        string
	Vendor          string
	CommandRaw      string
	CommandNorm     string
	RawOutput       string
	ContentHash     string
	FirstSeen       time.Time
	LastSeen        time.Time
	OccurrenceCount int
	Status          string
}

// PendingRule represents a row in the pending_rules table.
type PendingRule struct {
	ID              int
	Vendor          string
	CommandPattern  string
	ModelPattern    string // regex for device model, default ".*"
	OSPattern       string // regex for OS version, default ".*"
	OutputType      string
	SchemaYAML      string
	GoCodeDraft     string
	SampleInputs    string
	ExpectedOutputs string
	Confidence      float64
	OccurrenceCount int
	Status          string
	ApprovedBy      string
	ApprovedAt      *time.Time
	PRURL           string
	MergedAt        *time.Time
	GoFilePath      string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// RuleTestCase represents a row in the rule_test_cases table.
type RuleTestCase struct {
	ID          int
	RuleID      int
	Description string
	Input       string
	Expected    string
	CreatedAt   time.Time
}

// UpsertUnknownOutput inserts or increments occurrence_count for a duplicate.
func (db *DB) UpsertUnknownOutput(u UnknownOutput) error {
	_, err := db.Exec(`
		INSERT INTO unknown_outputs (device_id, vendor, command_raw, command_norm, raw_output, content_hash)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(vendor, command_norm, content_hash) DO UPDATE SET
			occurrence_count = occurrence_count + 1,
			last_seen = CURRENT_TIMESTAMP`,
		u.DeviceID, u.Vendor, u.CommandRaw, u.CommandNorm, u.RawOutput, u.ContentHash)
	return err
}

// ListUnknownOutputs returns outputs filtered by vendor and status, highest occurrence first.
func (db *DB) ListUnknownOutputs(vendor, status string, limit int) ([]UnknownOutput, error) {
	q := `SELECT id, device_id, vendor, command_raw, command_norm, raw_output, content_hash,
                 first_seen, last_seen, occurrence_count, status
          FROM unknown_outputs WHERE 1=1`
	var args []any
	if vendor != "" {
		q += " AND vendor = ?"
		args = append(args, vendor)
	}
	if status != "" {
		q += " AND status = ?"
		args = append(args, status)
	}
	q += " ORDER BY occurrence_count DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UnknownOutput
	for rows.Next() {
		var u UnknownOutput
		if err := rows.Scan(&u.ID, &u.DeviceID, &u.Vendor, &u.CommandRaw, &u.CommandNorm,
			&u.RawOutput, &u.ContentHash, &u.FirstSeen, &u.LastSeen, &u.OccurrenceCount, &u.Status); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateUnknownOutputStatus sets status for all outputs matching (vendor, command_norm).
func (db *DB) UpdateUnknownOutputStatus(vendor, commandNorm, status string) error {
	_, err := db.Exec(`UPDATE unknown_outputs SET status = ? WHERE vendor = ? AND command_norm = ?`,
		status, vendor, commandNorm)
	return err
}

// GetUnknownOutputByID fetches a single unknown output by its primary key.
func (db *DB) GetUnknownOutputByID(id int) (UnknownOutput, error) {
	var u UnknownOutput
	err := db.QueryRow(`
		SELECT id, device_id, vendor, command_raw, command_norm, raw_output, content_hash,
		       first_seen, last_seen, occurrence_count, status
		FROM unknown_outputs WHERE id = ?`, id).Scan(
		&u.ID, &u.DeviceID, &u.Vendor, &u.CommandRaw, &u.CommandNorm,
		&u.RawOutput, &u.ContentHash, &u.FirstSeen, &u.LastSeen, &u.OccurrenceCount, &u.Status)
	return u, err
}

// CreatePendingRule inserts a new rule and returns its ID.
func (db *DB) CreatePendingRule(r PendingRule) (int, error) {
	if r.ModelPattern == "" {
		r.ModelPattern = ".*"
	}
	if r.OSPattern == "" {
		r.OSPattern = ".*"
	}
	res, err := db.Exec(`
		INSERT INTO pending_rules (vendor, command_pattern, model_pattern, os_pattern, output_type, schema_yaml, go_code_draft,
			sample_inputs, expected_outputs, confidence, occurrence_count, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Vendor, r.CommandPattern, r.ModelPattern, r.OSPattern, r.OutputType, r.SchemaYAML, r.GoCodeDraft,
		r.SampleInputs, r.ExpectedOutputs, r.Confidence, r.OccurrenceCount, r.Status)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

// GetPendingRule fetches a single rule by ID.
func (db *DB) GetPendingRule(id int) (PendingRule, error) {
	var r PendingRule
	err := db.QueryRow(`
		SELECT id, vendor, command_pattern, COALESCE(model_pattern,'.*'), COALESCE(os_pattern,'.*'), output_type,
			COALESCE(schema_yaml,''), COALESCE(go_code_draft,''),
			sample_inputs, COALESCE(expected_outputs,''), COALESCE(confidence,0),
			COALESCE(occurrence_count,0), status, COALESCE(approved_by,''), approved_at,
			COALESCE(pr_url,''), merged_at, COALESCE(go_file_path,''),
			created_at, updated_at
		FROM pending_rules WHERE id = ?`, id).Scan(
		&r.ID, &r.Vendor, &r.CommandPattern, &r.ModelPattern, &r.OSPattern, &r.OutputType,
		&r.SchemaYAML, &r.GoCodeDraft, &r.SampleInputs, &r.ExpectedOutputs, &r.Confidence,
		&r.OccurrenceCount, &r.Status, &r.ApprovedBy, &r.ApprovedAt, &r.PRURL, &r.MergedAt, &r.GoFilePath,
		&r.CreatedAt, &r.UpdatedAt)
	return r, err
}

// ListPendingRules returns rules filtered by status, sorted by occurrence_count DESC.
func (db *DB) ListPendingRules(status string, limit int) ([]PendingRule, error) {
	q := `SELECT id, vendor, command_pattern, COALESCE(model_pattern,'.*'), COALESCE(os_pattern,'.*'),
		output_type, COALESCE(confidence,0),
		COALESCE(occurrence_count,0), status, created_at
          FROM pending_rules WHERE 1=1`
	var args []any
	if status != "" {
		q += " AND status = ?"
		args = append(args, status)
	}
	q += " ORDER BY occurrence_count DESC, created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingRule
	for rows.Next() {
		var r PendingRule
		if err := rows.Scan(&r.ID, &r.Vendor, &r.CommandPattern, &r.ModelPattern, &r.OSPattern,
			&r.OutputType, &r.Confidence, &r.OccurrenceCount, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpdatePendingRule updates schema/code/status fields of a rule.
func (db *DB) UpdatePendingRule(r PendingRule) error {
	_, err := db.Exec(`
		UPDATE pending_rules
		SET schema_yaml=?, go_code_draft=?, status=?, approved_by=?, pr_url=?, go_file_path=?
		WHERE id=?`,
		r.SchemaYAML, r.GoCodeDraft, r.Status, r.ApprovedBy, r.PRURL, r.GoFilePath, r.ID)
	return err
}

// ApprovePendingRule sets status=approved and records approver + timestamp.
func (db *DB) ApprovePendingRule(id int, approvedBy string, at time.Time) error {
	_, err := db.Exec(`
		UPDATE pending_rules SET status='approved', approved_by=?, approved_at=? WHERE id=?`,
		approvedBy, at, id)
	return err
}

// SetPendingRulePR records the PR URL after code generation.
func (db *DB) SetPendingRulePR(id int, prURL, goFilePath string) error {
	_, err := db.Exec(`UPDATE pending_rules SET pr_url=?, go_file_path=? WHERE id=?`,
		prURL, goFilePath, id)
	return err
}

// SetPendingRuleMerged marks a rule as merged.
func (db *DB) SetPendingRuleMerged(id int, at time.Time) error {
	_, err := db.Exec(`UPDATE pending_rules SET merged_at=? WHERE id=?`, at, id)
	return err
}

// CreateRuleTestCase inserts a test case.
func (db *DB) CreateRuleTestCase(tc RuleTestCase) (int, error) {
	res, err := db.Exec(`INSERT INTO rule_test_cases (rule_id, description, input, expected) VALUES (?,?,?,?)`,
		tc.RuleID, tc.Description, tc.Input, tc.Expected)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

// ListRuleTestCases returns all test cases for a rule.
func (db *DB) ListRuleTestCases(ruleID int) ([]RuleTestCase, error) {
	rows, err := db.Query(`
		SELECT id, rule_id, COALESCE(description,''), input, expected, created_at
		FROM rule_test_cases WHERE rule_id = ? ORDER BY id`, ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RuleTestCase
	for rows.Next() {
		var tc RuleTestCase
		if err := rows.Scan(&tc.ID, &tc.RuleID, &tc.Description, &tc.Input, &tc.Expected, &tc.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}

// DeleteRuleTestCase removes a test case by ID.
func (db *DB) DeleteRuleTestCase(id int) error {
	_, err := db.Exec(`DELETE FROM rule_test_cases WHERE id = ?`, id)
	return err
}

// CountRuleTestCases returns the number of test cases for a rule.
func (db *DB) CountRuleTestCases(ruleID int) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM rule_test_cases WHERE rule_id = ?`, ruleID).Scan(&n)
	return n, err
}

// DeletePendingRule removes a rule and its test cases by ID.
// It also resets the associated unknown_outputs back to "new" so they can be re-discovered.
func (db *DB) DeletePendingRule(id int) error {
	// Reset associated unknown_outputs to "new"
	db.Exec(`UPDATE unknown_outputs SET status = 'new'
		WHERE vendor || char(0) || command_norm IN (
			SELECT vendor || char(0) || command_pattern FROM pending_rules WHERE id = ?
		)`, id)
	// test cases have ON DELETE CASCADE, but let's be explicit
	db.Exec(`DELETE FROM rule_test_cases WHERE rule_id = ?`, id)
	_, err := db.Exec(`DELETE FROM pending_rules WHERE id = ?`, id)
	return err
}

// DeletePendingRulesByStatus removes all rules with the given status.
// It also resets the associated unknown_outputs back to "new" so they can be re-discovered.
func (db *DB) DeletePendingRulesByStatus(status string) (int, error) {
	// Reset associated unknown_outputs to "new" so they can be re-discovered
	db.Exec(`UPDATE unknown_outputs SET status = 'new'
		WHERE vendor || char(0) || command_norm IN (
			SELECT vendor || char(0) || command_pattern FROM pending_rules WHERE status = ?
		)`, status)
	// Delete associated test cases
	db.Exec(`DELETE FROM rule_test_cases WHERE rule_id IN (SELECT id FROM pending_rules WHERE status = ?)`, status)
	res, err := db.Exec(`DELETE FROM pending_rules WHERE status = ?`, status)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// SearchPendingRules searches rules by command pattern, vendor, or status.
func (db *DB) SearchPendingRules(query, vendor, status string, limit int) ([]PendingRule, error) {
	q := `SELECT id, vendor, command_pattern, COALESCE(model_pattern,'.*'), COALESCE(os_pattern,'.*'),
		output_type, COALESCE(confidence,0),
		COALESCE(occurrence_count,0), status, created_at
          FROM pending_rules WHERE 1=1`
	var args []any
	if query != "" {
		q += " AND command_pattern LIKE ?"
		args = append(args, "%"+query+"%")
	}
	if vendor != "" {
		q += " AND vendor = ?"
		args = append(args, vendor)
	}
	if status != "" {
		q += " AND status = ?"
		args = append(args, status)
	}
	q += " ORDER BY updated_at DESC, occurrence_count DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingRule
	for rows.Next() {
		var r PendingRule
		if err := rows.Scan(&r.ID, &r.Vendor, &r.CommandPattern, &r.ModelPattern, &r.OSPattern,
			&r.OutputType, &r.Confidence, &r.OccurrenceCount, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetPendingRuleByCommandNorm returns an existing draft/testing rule for (vendor, command_norm).
func (db *DB) GetPendingRuleByCommandNorm(vendor, commandNorm string) (PendingRule, error) {
	var r PendingRule
	err := db.QueryRow(`
		SELECT id, vendor, command_pattern, output_type, status
		FROM pending_rules
		WHERE vendor = ? AND command_pattern = ? AND status IN ('draft','testing')
		LIMIT 1`, vendor, commandNorm).Scan(
		&r.ID, &r.Vendor, &r.CommandPattern, &r.OutputType, &r.Status)
	if err == sql.ErrNoRows {
		return r, sql.ErrNoRows
	}
	return r, err
}

// ── Import History ───────────────────────────────────────────────────────

// ImportHistory represents a row in the import_history table.
type ImportHistory struct {
	ID               int
	Filename         string
	Vendor           string
	SourceType       string // "file", "paste", "manual"
	TotalCommands    int
	SelectedCommands int
	CreatedCount     int
	FailedCount      int
	SkippedCount     int
	ImportedAt       time.Time
}

// CreateImportHistory inserts a new import history record and returns its ID.
func (db *DB) CreateImportHistory(h ImportHistory) (int, error) {
	res, err := db.Exec(`
		INSERT INTO import_history (filename, vendor, source_type, total_commands, selected_commands,
			created_count, failed_count, skipped_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		h.Filename, h.Vendor, h.SourceType, h.TotalCommands, h.SelectedCommands,
		h.CreatedCount, h.FailedCount, h.SkippedCount)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

// ListImportHistory returns the most recent import history records.
func (db *DB) ListImportHistory(limit int) ([]ImportHistory, error) {
	rows, err := db.Query(`
		SELECT id, filename, vendor, source_type, total_commands, selected_commands,
			created_count, failed_count, skipped_count, imported_at
		FROM import_history
		ORDER BY imported_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ImportHistory
	for rows.Next() {
		var h ImportHistory
		if err := rows.Scan(&h.ID, &h.Filename, &h.Vendor, &h.SourceType,
			&h.TotalCommands, &h.SelectedCommands, &h.CreatedCount,
			&h.FailedCount, &h.SkippedCount, &h.ImportedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
