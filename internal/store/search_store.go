package store

import (
	"database/sql"

	"github.com/xavierli/nethelper/internal/model"
)

// SyncConfigFTS rebuilds the FTS index for config_snapshots.
func (db *DB) SyncConfigFTS() error {
	// Clear and rebuild
	db.Exec(`DELETE FROM fts_config`)
	_, err := db.Exec(`INSERT INTO fts_config(rowid, device_id, config_text, source_file)
		SELECT id, device_id, config_text, source_file FROM config_snapshots`)
	return err
}

// SearchConfig searches config snapshots using FTS5.
func (db *DB) SearchConfig(query string) ([]model.ConfigSnapshot, error) {
	rows, err := db.Query(`SELECT c.id, c.device_id, c.config_text, c.diff_from_prev, c.captured_at, c.source_file
		FROM config_snapshots c
		JOIN fts_config f ON c.id = f.rowid
		WHERE fts_config MATCH ?
		ORDER BY rank`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.ConfigSnapshot
	for rows.Next() {
		var cs model.ConfigSnapshot
		if err := rows.Scan(&cs.ID, &cs.DeviceID, &cs.ConfigText, &cs.DiffFromPrev, &cs.CapturedAt, &cs.SourceFile); err != nil {
			return nil, err
		}
		results = append(results, cs)
	}
	return results, rows.Err()
}

// InsertCommandReference inserts a command reference and syncs FTS.
func (db *DB) InsertCommandReference(ref model.CommandReference) (int, error) {
	result, err := db.Exec(`INSERT INTO command_references (vendor, command, description, example_output, parse_hint, source_url)
		VALUES (?, ?, ?, ?, ?, ?)`,
		ref.Vendor, ref.Command, ref.Description, ref.ExampleOutput, ref.ParseHint, ref.SourceURL)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	db.Exec(`INSERT INTO fts_commands(rowid, vendor, command, description) VALUES (?, ?, ?, ?)`,
		id, ref.Vendor, ref.Command, ref.Description)

	return int(id), nil
}

// SearchCommands searches command references using FTS5.
func (db *DB) SearchCommands(query string) ([]model.CommandReference, error) {
	rows, err := db.Query(`SELECT c.id, c.vendor, c.command, c.description, c.example_output, c.parse_hint, c.source_url
		FROM command_references c
		JOIN fts_commands f ON c.id = f.rowid
		WHERE fts_commands MATCH ?
		ORDER BY rank`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.CommandReference
	for rows.Next() {
		var r model.CommandReference
		if err := rows.Scan(&r.ID, &r.Vendor, &r.Command, &r.Description, &r.ExampleOutput, &r.ParseHint, &r.SourceURL); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ListCommandReferences lists all command references, optionally filtered by vendor.
func (db *DB) ListCommandReferences(vendor string) ([]model.CommandReference, error) {
	query := `SELECT id, vendor, command, description, example_output, parse_hint, source_url FROM command_references`
	var rows *sql.Rows
	var err error
	if vendor != "" {
		rows, err = db.Query(query+" WHERE vendor = ? ORDER BY command", vendor)
	} else {
		rows, err = db.Query(query+" ORDER BY vendor, command")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []model.CommandReference
	for rows.Next() {
		var r model.CommandReference
		if err := rows.Scan(&r.ID, &r.Vendor, &r.Command, &r.Description, &r.ExampleOutput, &r.ParseHint, &r.SourceURL); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
