package store

import "github.com/xavierli/nethelper/internal/model"

func (db *DB) InsertSRMappings(entries []model.SRMapping) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO sr_mappings (device_id, prefix, sid_index, sid_label, algorithm, flags, source, snapshot_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		if _, err := stmt.Exec(e.DeviceID, e.Prefix, e.SIDIndex, e.SIDLabel, e.Algorithm, e.Flags, e.Source, e.SnapshotID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) GetSRMappings(deviceID string, snapshotID int) ([]model.SRMapping, error) {
	rows, err := db.Query(`SELECT id, device_id, prefix, sid_index, sid_label, algorithm, flags, source, snapshot_id
		FROM sr_mappings WHERE device_id = ? AND snapshot_id = ? ORDER BY prefix`, deviceID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []model.SRMapping
	for rows.Next() {
		var e model.SRMapping
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.Prefix, &e.SIDIndex, &e.SIDLabel, &e.Algorithm, &e.Flags, &e.Source, &e.SnapshotID); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
