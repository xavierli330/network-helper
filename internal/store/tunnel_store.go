package store

import "github.com/xavierli/nethelper/internal/model"

func (db *DB) InsertTunnels(entries []model.TunnelInfo) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO mpls_te_tunnels (device_id, tunnel_name, type, destination, state, signaled_bw, explicit_path, actual_path, binding_sid, snapshot_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		if e.ExplicitPath == "" {
			e.ExplicitPath = "[]"
		}
		if e.ActualPath == "" {
			e.ActualPath = "[]"
		}
		if _, err := stmt.Exec(e.DeviceID, e.TunnelName, e.Type, e.Destination, e.State, e.SignaledBW, e.ExplicitPath, e.ActualPath, e.BindingSID, e.SnapshotID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (db *DB) GetTunnels(deviceID string, snapshotID int) ([]model.TunnelInfo, error) {
	rows, err := db.Query(`SELECT id, device_id, tunnel_name, type, destination, state, signaled_bw, explicit_path, actual_path, binding_sid, snapshot_id
		FROM mpls_te_tunnels WHERE device_id = ? AND snapshot_id = ? ORDER BY tunnel_name`, deviceID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []model.TunnelInfo
	for rows.Next() {
		var e model.TunnelInfo
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.TunnelName, &e.Type, &e.Destination, &e.State, &e.SignaledBW, &e.ExplicitPath, &e.ActualPath, &e.BindingSID, &e.SnapshotID); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
