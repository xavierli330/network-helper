package store

import (
	"database/sql"

	"github.com/xavierli/nethelper/internal/model"
)

func (db *DB) CreateSnapshot(s model.Snapshot) (int, error) {
	if s.Commands == "" {
		s.Commands = "[]"
	}
	var (
		result sql.Result
		err    error
	)
	if !s.CapturedAt.IsZero() {
		result, err = db.Exec(
			`INSERT INTO snapshots (device_id, source_file, commands, captured_at) VALUES (?, ?, ?, ?)`,
			s.DeviceID, s.SourceFile, s.Commands, s.CapturedAt,
		)
	} else {
		result, err = db.Exec(
			`INSERT INTO snapshots (device_id, source_file, commands) VALUES (?, ?, ?)`,
			s.DeviceID, s.SourceFile, s.Commands,
		)
	}
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func (db *DB) GetSnapshot(id int) (model.Snapshot, error) {
	var s model.Snapshot
	err := db.QueryRow(`SELECT id, device_id, captured_at, source_file, commands FROM snapshots WHERE id = ?`, id).
		Scan(&s.ID, &s.DeviceID, &s.CapturedAt, &s.SourceFile, &s.Commands)
	return s, err
}

func (db *DB) LatestSnapshotID(deviceID string) (int, error) {
	var id int
	err := db.QueryRow(`SELECT id FROM snapshots WHERE device_id = ? ORDER BY id DESC LIMIT 1`, deviceID).Scan(&id)
	return id, err
}
