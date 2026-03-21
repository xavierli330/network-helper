package store

import "github.com/xavierli/nethelper/internal/model"

func (db *DB) InsertFIBEntries(entries []model.FIBEntry) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO fib_entries (device_id, vrf, prefix, mask_len, next_hop, outgoing_interface, label_action, out_label, tunnel_id, snapshot_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		if e.VRF == "" {
			e.VRF = "default"
		}
		if _, err := stmt.Exec(e.DeviceID, e.VRF, e.Prefix, e.MaskLen, e.NextHop, e.OutgoingInterface, e.LabelAction, e.OutLabel, e.TunnelID, e.SnapshotID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) GetFIBEntries(deviceID string, snapshotID int) ([]model.FIBEntry, error) {
	rows, err := db.Query(`SELECT id, device_id, vrf, prefix, mask_len, next_hop, outgoing_interface, label_action, out_label, tunnel_id, snapshot_id
		FROM fib_entries WHERE device_id = ? AND snapshot_id = ? ORDER BY prefix, mask_len`, deviceID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []model.FIBEntry
	for rows.Next() {
		var e model.FIBEntry
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.VRF, &e.Prefix, &e.MaskLen, &e.NextHop, &e.OutgoingInterface, &e.LabelAction, &e.OutLabel, &e.TunnelID, &e.SnapshotID); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
