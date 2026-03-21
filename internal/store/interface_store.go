package store

import "github.com/xavierli/nethelper/internal/model"

func (db *DB) UpsertInterface(i model.Interface) error {
	_, err := db.Exec(`
		INSERT INTO interfaces (id, device_id, name, type, status, ip_address, mask, vlan, bandwidth, description, parent_id, last_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, type=excluded.type, status=excluded.status,
			ip_address=excluded.ip_address, mask=excluded.mask, vlan=excluded.vlan,
			bandwidth=excluded.bandwidth, description=excluded.description,
			parent_id=excluded.parent_id, last_updated=excluded.last_updated`,
		i.ID, i.DeviceID, i.Name, string(i.Type), i.Status, i.IPAddress, i.Mask, i.VLAN, i.Bandwidth, i.Description, i.ParentID, i.LastUpdated)
	return err
}

func (db *DB) GetInterfaces(deviceID string) ([]model.Interface, error) {
	rows, err := db.Query(`SELECT id, device_id, name, type, status, ip_address, mask, vlan, bandwidth, description, parent_id, last_updated
		FROM interfaces WHERE device_id = ? ORDER BY name`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ifaces []model.Interface
	for rows.Next() {
		var i model.Interface
		var ifType string
		if err := rows.Scan(&i.ID, &i.DeviceID, &i.Name, &ifType, &i.Status, &i.IPAddress, &i.Mask, &i.VLAN, &i.Bandwidth, &i.Description, &i.ParentID, &i.LastUpdated); err != nil {
			return nil, err
		}
		i.Type = model.InterfaceType(ifType)
		ifaces = append(ifaces, i)
	}
	return ifaces, rows.Err()
}
