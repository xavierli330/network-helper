package store

import (
	"github.com/xavierli/nethelper/internal/model"
)

func (db *DB) InsertTroubleshootLog(log model.TroubleshootLog) (int, error) {
	result, err := db.Exec(`INSERT INTO troubleshoot_logs (device_id, symptom, commands_used, findings, resolution, tags)
		VALUES (?, ?, ?, ?, ?, ?)`,
		log.DeviceID, log.Symptom, log.CommandsUsed, log.Findings, log.Resolution, log.Tags)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Sync to FTS5 index
	db.Exec(`INSERT INTO fts_troubleshoot(rowid, symptom, commands_used, findings, resolution, tags)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, log.Symptom, log.CommandsUsed, log.Findings, log.Resolution, log.Tags)

	return int(id), nil
}

func (db *DB) ListTroubleshootLogs(limit, offset int) ([]model.TroubleshootLog, error) {
	rows, err := db.Query(`SELECT id, device_id, symptom, commands_used, findings, resolution, tags, created_at
		FROM troubleshoot_logs ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []model.TroubleshootLog
	for rows.Next() {
		var l model.TroubleshootLog
		if err := rows.Scan(&l.ID, &l.DeviceID, &l.Symptom, &l.CommandsUsed, &l.Findings, &l.Resolution, &l.Tags, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (db *DB) SearchTroubleshootLogs(query string) ([]model.TroubleshootLog, error) {
	rows, err := db.Query(`SELECT t.id, t.device_id, t.symptom, t.commands_used, t.findings, t.resolution, t.tags, t.created_at
		FROM troubleshoot_logs t
		JOIN fts_troubleshoot f ON t.id = f.rowid
		WHERE fts_troubleshoot MATCH ?
		ORDER BY rank`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []model.TroubleshootLog
	for rows.Next() {
		var l model.TroubleshootLog
		if err := rows.Scan(&l.ID, &l.DeviceID, &l.Symptom, &l.CommandsUsed, &l.Findings, &l.Resolution, &l.Tags, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}
