package store

import "github.com/xavierli/nethelper/internal/model"

func (db *DB) InsertRIBEntries(entries []model.RIBEntry) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO rib_entries (device_id, vrf, prefix, mask_len, protocol, next_hop, outgoing_interface, preference, metric, tag, snapshot_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		if e.VRF == "" {
			e.VRF = "default"
		}
		if _, err := stmt.Exec(e.DeviceID, e.VRF, e.Prefix, e.MaskLen, e.Protocol, e.NextHop, e.OutgoingInterface, e.Preference, e.Metric, e.Tag, e.SnapshotID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) GetRIBEntries(deviceID string, snapshotID int) ([]model.RIBEntry, error) {
	rows, err := db.Query(`SELECT id, device_id, vrf, prefix, mask_len, protocol, next_hop, outgoing_interface, preference, metric, tag, snapshot_id
		FROM rib_entries WHERE device_id = ? AND snapshot_id = ? ORDER BY prefix, mask_len`, deviceID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []model.RIBEntry
	for rows.Next() {
		var e model.RIBEntry
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.VRF, &e.Prefix, &e.MaskLen, &e.Protocol, &e.NextHop, &e.OutgoingInterface, &e.Preference, &e.Metric, &e.Tag, &e.SnapshotID); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
