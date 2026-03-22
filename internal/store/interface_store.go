package store

import "github.com/xavierli/nethelper/internal/model"

func (db *DB) UpsertInterface(i model.Interface) error {
	_, err := db.Exec(`
		INSERT INTO interfaces (id, device_id, name, type, status, ip_address, mask, vlan, bandwidth, description, parent_id, last_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  name        = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.name        ELSE interfaces.name        END,
		  type        = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.type        ELSE interfaces.type        END,
		  status      = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.status      ELSE interfaces.status      END,
		  ip_address  = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.ip_address  ELSE interfaces.ip_address  END,
		  mask        = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.mask        ELSE interfaces.mask        END,
		  vlan        = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.vlan        ELSE interfaces.vlan        END,
		  bandwidth   = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.bandwidth   ELSE interfaces.bandwidth   END,
		  description = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.description ELSE interfaces.description END,
		  parent_id   = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.parent_id   ELSE interfaces.parent_id   END,
		  last_updated = CASE WHEN excluded.last_updated >= interfaces.last_updated THEN excluded.last_updated ELSE interfaces.last_updated END`,
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
