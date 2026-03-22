package store

import "github.com/xavierli/nethelper/internal/model"

func (db *DB) InsertBGPPeers(peers []model.BGPPeer) error {
	for _, p := range peers {
		_, err := db.Exec(`INSERT INTO bgp_peers
			(device_id, vrf, local_as, peer_ip, remote_as, peer_group, description,
			 update_source, ebgp_multihop, bfd_enabled, shutdown, address_family,
			 import_policy, export_policy, advertise_community, next_hop_self,
			 soft_reconfig, enabled, snapshot_id)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.DeviceID, p.VRF, p.LocalAS, p.PeerIP, p.RemoteAS, p.PeerGroup,
			p.Description, p.UpdateSource, p.EBGPMultihop, p.BFDEnabled, p.Shutdown,
			p.AddressFamily, p.ImportPolicy, p.ExportPolicy, p.AdvertiseCommunity,
			p.NextHopSelf, p.SoftReconfig, p.Enabled, p.SnapshotID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) GetBGPPeers(deviceID string, snapshotID int) ([]model.BGPPeer, error) {
	rows, err := db.Query(`SELECT id, device_id, vrf, local_as, peer_ip, remote_as,
		peer_group, description, update_source, ebgp_multihop, bfd_enabled, shutdown,
		address_family, import_policy, export_policy, advertise_community, next_hop_self,
		soft_reconfig, enabled, snapshot_id
		FROM bgp_peers WHERE device_id = ? AND snapshot_id = ?
		ORDER BY vrf, address_family, peer_ip`, deviceID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var peers []model.BGPPeer
	for rows.Next() {
		var p model.BGPPeer
		if err := rows.Scan(&p.ID, &p.DeviceID, &p.VRF, &p.LocalAS, &p.PeerIP,
			&p.RemoteAS, &p.PeerGroup, &p.Description, &p.UpdateSource, &p.EBGPMultihop,
			&p.BFDEnabled, &p.Shutdown, &p.AddressFamily, &p.ImportPolicy, &p.ExportPolicy,
			&p.AdvertiseCommunity, &p.NextHopSelf, &p.SoftReconfig, &p.Enabled,
			&p.SnapshotID); err != nil {
			return nil, err
		}
		peers = append(peers, p)
	}
	return peers, rows.Err()
}

func (db *DB) InsertVRFInstances(vrfs []model.VRFInstance) error {
	for _, v := range vrfs {
		_, err := db.Exec(`INSERT INTO vrf_instances
			(device_id, vrf_name, rd, import_rt, export_rt, import_policy, export_policy,
			 tunnel_policy, label_mode, address_family, snapshot_id)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			v.DeviceID, v.VRFName, v.RD, v.ImportRT, v.ExportRT, v.ImportPolicy,
			v.ExportPolicy, v.TunnelPolicy, v.LabelMode, v.AddressFamily, v.SnapshotID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) GetVRFInstances(deviceID string) ([]model.VRFInstance, error) {
	rows, err := db.Query(`SELECT id, device_id, vrf_name, rd, import_rt, export_rt,
		import_policy, export_policy, tunnel_policy, label_mode, address_family, snapshot_id
		FROM vrf_instances WHERE device_id = ? ORDER BY vrf_name`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var vrfs []model.VRFInstance
	for rows.Next() {
		var v model.VRFInstance
		if err := rows.Scan(&v.ID, &v.DeviceID, &v.VRFName, &v.RD, &v.ImportRT,
			&v.ExportRT, &v.ImportPolicy, &v.ExportPolicy, &v.TunnelPolicy,
			&v.LabelMode, &v.AddressFamily, &v.SnapshotID); err != nil {
			return nil, err
		}
		vrfs = append(vrfs, v)
	}
	return vrfs, rows.Err()
}

func (db *DB) InsertRoutePolicy(rp model.RoutePolicy) (int, error) {
	result, err := db.Exec(`INSERT INTO route_policies
		(device_id, policy_name, vendor_type, raw_text, snapshot_id)
		VALUES (?,?,?,?,?)`,
		rp.DeviceID, rp.PolicyName, rp.VendorType, rp.RawText, rp.SnapshotID)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	policyID := int(id)

	for _, n := range rp.Nodes {
		_, err := db.Exec(`INSERT INTO route_policy_nodes
			(policy_id, sequence, term_name, action, match_clauses, apply_clauses)
			VALUES (?,?,?,?,?,?)`,
			policyID, n.Sequence, n.TermName, n.Action, n.MatchClauses, n.ApplyClauses)
		if err != nil {
			return policyID, err
		}
	}
	return policyID, nil
}

func (db *DB) GetRoutePolicies(deviceID string) ([]model.RoutePolicy, error) {
	rows, err := db.Query(`SELECT id, device_id, policy_name, vendor_type, raw_text, snapshot_id
		FROM route_policies WHERE device_id = ? ORDER BY policy_name`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var policies []model.RoutePolicy
	for rows.Next() {
		var rp model.RoutePolicy
		if err := rows.Scan(&rp.ID, &rp.DeviceID, &rp.PolicyName, &rp.VendorType,
			&rp.RawText, &rp.SnapshotID); err != nil {
			return nil, err
		}
		policies = append(policies, rp)
	}
	return policies, rows.Err()
}
