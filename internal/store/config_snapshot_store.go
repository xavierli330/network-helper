package store

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/xavierli/nethelper/internal/model"
)

func (db *DB) InsertConfigSnapshot(cs model.ConfigSnapshot) (int, error) {
	// Compute content hash
	hash := sha256.Sum256([]byte(cs.ConfigText))
	hashStr := hex.EncodeToString(hash[:])

	// Check if identical config already exists for this device
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM config_snapshots WHERE device_id = ? AND content_hash = ?`,
		cs.DeviceID, hashStr).Scan(&count)
	if count > 0 {
		return 0, nil // skip duplicate
	}

	if !cs.CapturedAt.IsZero() {
		result, err := db.Exec(`INSERT INTO config_snapshots (device_id, config_text, diff_from_prev, captured_at, source_file, format, content_hash) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			cs.DeviceID, cs.ConfigText, cs.DiffFromPrev, cs.CapturedAt, cs.SourceFile, cs.Format, hashStr)
		if err != nil {
			return 0, err
		}
		id, err := result.LastInsertId()
		return int(id), err
	}
	result, err := db.Exec(`INSERT INTO config_snapshots (device_id, config_text, diff_from_prev, source_file, format, content_hash) VALUES (?, ?, ?, ?, ?, ?)`,
		cs.DeviceID, cs.ConfigText, cs.DiffFromPrev, cs.SourceFile, cs.Format, hashStr)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func (db *DB) GetConfigSnapshots(deviceID string) ([]model.ConfigSnapshot, error) {
	rows, err := db.Query(`SELECT id, device_id, config_text, diff_from_prev, captured_at, source_file, format
		FROM config_snapshots WHERE device_id = ? ORDER BY captured_at DESC`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var snapshots []model.ConfigSnapshot
	for rows.Next() {
		var cs model.ConfigSnapshot
		if err := rows.Scan(&cs.ID, &cs.DeviceID, &cs.ConfigText, &cs.DiffFromPrev, &cs.CapturedAt, &cs.SourceFile, &cs.Format); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, cs)
	}
	return snapshots, rows.Err()
}
