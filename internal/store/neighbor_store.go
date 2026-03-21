package store

import "github.com/xavierli/nethelper/internal/model"

func (db *DB) InsertNeighbors(entries []model.NeighborInfo) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO protocol_neighbors (device_id, protocol, local_id, remote_id, local_interface, remote_address, state, area_id, as_number, uptime, snapshot_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		if _, err := stmt.Exec(e.DeviceID, e.Protocol, e.LocalID, e.RemoteID, e.LocalInterface, e.RemoteAddress, e.State, e.AreaID, e.ASNumber, e.Uptime, e.SnapshotID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) GetNeighbors(deviceID string, snapshotID int) ([]model.NeighborInfo, error) {
	rows, err := db.Query(`SELECT id, device_id, protocol, local_id, remote_id, local_interface, remote_address, state, area_id, as_number, uptime, snapshot_id
		FROM protocol_neighbors WHERE device_id = ? AND snapshot_id = ? ORDER BY protocol, remote_id`, deviceID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []model.NeighborInfo
	for rows.Next() {
		var e model.NeighborInfo
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.Protocol, &e.LocalID, &e.RemoteID, &e.LocalInterface, &e.RemoteAddress, &e.State, &e.AreaID, &e.ASNumber, &e.Uptime, &e.SnapshotID); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
