package store

import (
	"database/sql"
	"fmt"
	"github.com/xavierli/nethelper/internal/model"
)

func (db *DB) UpsertDevice(d model.Device) error {
	_, err := db.Exec(`
		INSERT INTO devices (id, hostname, vendor, model, os_version, mgmt_ip, router_id, mpls_lsr_id, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			hostname=excluded.hostname, vendor=excluded.vendor, model=excluded.model,
			os_version=excluded.os_version, mgmt_ip=excluded.mgmt_ip,
			router_id=excluded.router_id, mpls_lsr_id=excluded.mpls_lsr_id,
			last_seen=excluded.last_seen`,
		d.ID, d.Hostname, d.Vendor, d.Model, d.OSVersion, d.MgmtIP, d.RouterID, d.MPLSLsrID, d.LastSeen)
	return err
}

func (db *DB) GetDevice(id string) (model.Device, error) {
	var d model.Device
	err := db.QueryRow(`SELECT id, hostname, vendor, model, os_version, mgmt_ip, router_id, mpls_lsr_id, last_seen
		FROM devices WHERE id = ?`, id).Scan(
		&d.ID, &d.Hostname, &d.Vendor, &d.Model, &d.OSVersion, &d.MgmtIP, &d.RouterID, &d.MPLSLsrID, &d.LastSeen)
	if err == sql.ErrNoRows {
		return d, fmt.Errorf("device %q not found", id)
	}
	return d, err
}

func (db *DB) ListDevices() ([]model.Device, error) {
	rows, err := db.Query(`SELECT id, hostname, vendor, model, os_version, mgmt_ip, router_id, mpls_lsr_id, last_seen
		FROM devices ORDER BY hostname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devices []model.Device
	for rows.Next() {
		var d model.Device
		if err := rows.Scan(&d.ID, &d.Hostname, &d.Vendor, &d.Model, &d.OSVersion, &d.MgmtIP, &d.RouterID, &d.MPLSLsrID, &d.LastSeen); err != nil {
			return nil, err
		}
		devices = append(devices, d)
	}
	return devices, rows.Err()
}
