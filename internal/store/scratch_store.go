package store

import "github.com/xavierli/nethelper/internal/model"

const defaultScratchLimit = 200

// InsertScratch adds an entry to the scratch pad and enforces FIFO eviction.
func (db *DB) InsertScratch(entry model.ScratchEntry) (int, error) {
	result, err := db.Exec(`INSERT INTO scratch_entries (device_id, category, query, content) VALUES (?, ?, ?, ?)`,
		entry.DeviceID, entry.Category, entry.Query, entry.Content)
	if err != nil {
		return 0, err
	}
	id, _ := result.LastInsertId()

	// FIFO eviction: keep only the most recent N entries
	db.Exec(`DELETE FROM scratch_entries WHERE id NOT IN (
		SELECT id FROM scratch_entries ORDER BY id DESC LIMIT ?)`, defaultScratchLimit)

	return int(id), nil
}

// ListScratch returns recent scratch entries, optionally filtered by device or category.
func (db *DB) ListScratch(deviceID, category string, limit int) ([]model.ScratchEntry, error) {
	if limit <= 0 {
		limit = 20
	}

	query := `SELECT id, device_id, category, query, content, created_at FROM scratch_entries WHERE 1=1`
	var args []any
	if deviceID != "" {
		query += ` AND device_id = ?`
		args = append(args, deviceID)
	}
	if category != "" {
		query += ` AND category = ?`
		args = append(args, category)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.ScratchEntry
	for rows.Next() {
		var e model.ScratchEntry
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.Category, &e.Query, &e.Content, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetScratch returns a single scratch entry by ID.
func (db *DB) GetScratch(id int) (model.ScratchEntry, error) {
	var e model.ScratchEntry
	err := db.QueryRow(`SELECT id, device_id, category, query, content, created_at FROM scratch_entries WHERE id = ?`, id).
		Scan(&e.ID, &e.DeviceID, &e.Category, &e.Query, &e.Content, &e.CreatedAt)
	return e, err
}

// ClearScratch removes all scratch entries, optionally filtered by device.
func (db *DB) ClearScratch(deviceID string) error {
	if deviceID != "" {
		_, err := db.Exec(`DELETE FROM scratch_entries WHERE device_id = ?`, deviceID)
		return err
	}
	_, err := db.Exec(`DELETE FROM scratch_entries`)
	return err
}
