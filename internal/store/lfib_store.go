package store

import "github.com/xavierli/nethelper/internal/model"

func (db *DB) InsertLFIBEntries(entries []model.LFIBEntry) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO lfib_entries (device_id, in_label, action, out_label, next_hop, outgoing_interface, fec, protocol, snapshot_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		if _, err := stmt.Exec(e.DeviceID, e.InLabel, e.Action, e.OutLabel, e.NextHop, e.OutgoingInterface, e.FEC, e.Protocol, e.SnapshotID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) GetLFIBEntries(deviceID string, snapshotID int) ([]model.LFIBEntry, error) {
	rows, err := db.Query(`SELECT id, device_id, in_label, action, out_label, next_hop, outgoing_interface, fec, protocol, snapshot_id
		FROM lfib_entries WHERE device_id = ? AND snapshot_id = ? ORDER BY in_label`, deviceID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []model.LFIBEntry
	for rows.Next() {
		var e model.LFIBEntry
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.InLabel, &e.Action, &e.OutLabel, &e.NextHop, &e.OutgoingInterface, &e.FEC, &e.Protocol, &e.SnapshotID); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
