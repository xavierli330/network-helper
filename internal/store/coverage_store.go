package store

import (
	"database/sql"
	"encoding/json"
	"time"
)

// CoverageCheck represents a stored self-check report for one device.
type CoverageCheck struct {
	ID           int       `json:"id"`
	DeviceID     string    `json:"device_id"`
	Vendor       string    `json:"vendor"`
	TotalCount   int       `json:"total_count"`
	CoveredCount int       `json:"covered_count"`
	CoveragePct  float64   `json:"coverage_pct"`
	ItemsJSON    string    `json:"items_json"` // JSON-encoded []CoverageItemRow
	CheckedAt    time.Time `json:"checked_at"`
}

// CoverageItemRow is the DB-friendly version of parser.CoverageItem.
type CoverageItemRow struct {
	Command  string `json:"command"`
	Category string `json:"category"`
	Reason   string `json:"reason"`
	Priority string `json:"priority"`
	Status   string `json:"status"`   // "covered" or "uncovered"
	CmdType  string `json:"cmd_type"`
}

// InsertCoverageCheck stores a coverage check report. On conflict (same device),
// it replaces the existing row to keep only the latest result.
func (db *DB) InsertCoverageCheck(cc CoverageCheck) (int, error) {
	result, err := db.Exec(`INSERT OR REPLACE INTO coverage_checks
		(device_id, vendor, total_count, covered_count, coverage_pct, items_json, checked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		cc.DeviceID, cc.Vendor, cc.TotalCount, cc.CoveredCount, cc.CoveragePct, cc.ItemsJSON, cc.CheckedAt)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

// GetCoverageCheck retrieves the latest coverage check for a device.
func (db *DB) GetCoverageCheck(deviceID string) (*CoverageCheck, error) {
	var cc CoverageCheck
	err := db.QueryRow(`SELECT id, device_id, vendor, total_count, covered_count, coverage_pct, items_json, checked_at
		FROM coverage_checks WHERE device_id = ? ORDER BY checked_at DESC LIMIT 1`, deviceID).
		Scan(&cc.ID, &cc.DeviceID, &cc.Vendor, &cc.TotalCount, &cc.CoveredCount, &cc.CoveragePct, &cc.ItemsJSON, &cc.CheckedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cc, nil
}

// ListCoverageChecks returns the latest coverage check per device.
func (db *DB) ListCoverageChecks() ([]CoverageCheck, error) {
	rows, err := db.Query(`SELECT id, device_id, vendor, total_count, covered_count, coverage_pct, items_json, checked_at
		FROM coverage_checks ORDER BY checked_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CoverageCheck
	for rows.Next() {
		var cc CoverageCheck
		if err := rows.Scan(&cc.ID, &cc.DeviceID, &cc.Vendor, &cc.TotalCount, &cc.CoveredCount, &cc.CoveragePct, &cc.ItemsJSON, &cc.CheckedAt); err != nil {
			return nil, err
		}
		result = append(result, cc)
	}
	return result, rows.Err()
}

// DeleteCoverageCheck removes a coverage check by device ID.
func (db *DB) DeleteCoverageCheck(deviceID string) error {
	_, err := db.Exec(`DELETE FROM coverage_checks WHERE device_id = ?`, deviceID)
	return err
}

// ParseCoverageItems deserializes the items_json into a typed slice.
func ParseCoverageItems(itemsJSON string) ([]CoverageItemRow, error) {
	var items []CoverageItemRow
	if err := json.Unmarshal([]byte(itemsJSON), &items); err != nil {
		return nil, err
	}
	return items, nil
}
