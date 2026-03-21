package store

import "github.com/xavierli/nethelper/internal/model"

func (db *DB) InsertConfigSnapshot(cs model.ConfigSnapshot) (int, error) {
	if !cs.CapturedAt.IsZero() {
		result, err := db.Exec(`INSERT INTO config_snapshots (device_id, config_text, diff_from_prev, captured_at, source_file, format) VALUES (?, ?, ?, ?, ?, ?)`,
			cs.DeviceID, cs.ConfigText, cs.DiffFromPrev, cs.CapturedAt, cs.SourceFile, cs.Format)
		if err != nil {
			return 0, err
		}
		id, err := result.LastInsertId()
		return int(id), err
	}
	result, err := db.Exec(`INSERT INTO config_snapshots (device_id, config_text, diff_from_prev, source_file, format) VALUES (?, ?, ?, ?, ?)`,
		cs.DeviceID, cs.ConfigText, cs.DiffFromPrev, cs.SourceFile, cs.Format)
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
