package store

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS devices (
		id TEXT PRIMARY KEY,
		hostname TEXT NOT NULL,
		vendor TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		os_version TEXT NOT NULL DEFAULT '',
		mgmt_ip TEXT NOT NULL DEFAULT '',
		router_id TEXT NOT NULL DEFAULT '',
		mpls_lsr_id TEXT NOT NULL DEFAULT '',
		last_seen TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	`CREATE TABLE IF NOT EXISTS interfaces (
		id TEXT PRIMARY KEY,
		device_id TEXT NOT NULL REFERENCES devices(id),
		name TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT 'physical',
		status TEXT NOT NULL DEFAULT 'down',
		ip_address TEXT NOT NULL DEFAULT '',
		mask TEXT NOT NULL DEFAULT '',
		vlan INTEGER NOT NULL DEFAULT 0,
		bandwidth TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		parent_id TEXT NOT NULL DEFAULT '',
		last_updated TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_interfaces_device ON interfaces(device_id)`,

	`CREATE TABLE IF NOT EXISTS snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL REFERENCES devices(id),
		captured_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		source_file TEXT NOT NULL DEFAULT '',
		commands TEXT NOT NULL DEFAULT '[]'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_snapshots_device ON snapshots(device_id)`,

	`CREATE TABLE IF NOT EXISTS rib_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		vrf TEXT NOT NULL DEFAULT 'default',
		prefix TEXT NOT NULL,
		mask_len INTEGER NOT NULL,
		protocol TEXT NOT NULL DEFAULT '',
		next_hop TEXT NOT NULL DEFAULT '',
		outgoing_interface TEXT NOT NULL DEFAULT '',
		preference INTEGER NOT NULL DEFAULT 0,
		metric INTEGER NOT NULL DEFAULT 0,
		tag INTEGER NOT NULL DEFAULT 0,
		snapshot_id INTEGER NOT NULL REFERENCES snapshots(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_rib_device ON rib_entries(device_id)`,
	`CREATE INDEX IF NOT EXISTS idx_rib_snapshot ON rib_entries(snapshot_id)`,
	`CREATE INDEX IF NOT EXISTS idx_rib_prefix ON rib_entries(prefix, mask_len)`,

	`CREATE TABLE IF NOT EXISTS fib_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		vrf TEXT NOT NULL DEFAULT 'default',
		prefix TEXT NOT NULL,
		mask_len INTEGER NOT NULL,
		next_hop TEXT NOT NULL DEFAULT '',
		outgoing_interface TEXT NOT NULL DEFAULT '',
		label_action TEXT NOT NULL DEFAULT 'none',
		out_label TEXT NOT NULL DEFAULT '',
		tunnel_id TEXT NOT NULL DEFAULT '',
		snapshot_id INTEGER NOT NULL REFERENCES snapshots(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_fib_device ON fib_entries(device_id)`,
	`CREATE INDEX IF NOT EXISTS idx_fib_snapshot ON fib_entries(snapshot_id)`,

	`CREATE TABLE IF NOT EXISTS lfib_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		in_label INTEGER NOT NULL,
		action TEXT NOT NULL DEFAULT '',
		out_label TEXT NOT NULL DEFAULT '',
		next_hop TEXT NOT NULL DEFAULT '',
		outgoing_interface TEXT NOT NULL DEFAULT '',
		fec TEXT NOT NULL DEFAULT '',
		protocol TEXT NOT NULL DEFAULT '',
		snapshot_id INTEGER NOT NULL REFERENCES snapshots(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_lfib_device ON lfib_entries(device_id)`,
	`CREATE INDEX IF NOT EXISTS idx_lfib_snapshot ON lfib_entries(snapshot_id)`,
	`CREATE INDEX IF NOT EXISTS idx_lfib_label ON lfib_entries(in_label)`,

	`CREATE TABLE IF NOT EXISTS protocol_neighbors (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		protocol TEXT NOT NULL,
		local_id TEXT NOT NULL DEFAULT '',
		remote_id TEXT NOT NULL DEFAULT '',
		local_interface TEXT NOT NULL DEFAULT '',
		remote_address TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT '',
		area_id TEXT NOT NULL DEFAULT '',
		as_number INTEGER NOT NULL DEFAULT 0,
		uptime TEXT NOT NULL DEFAULT '',
		snapshot_id INTEGER NOT NULL REFERENCES snapshots(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_neighbors_device ON protocol_neighbors(device_id)`,
	`CREATE INDEX IF NOT EXISTS idx_neighbors_snapshot ON protocol_neighbors(snapshot_id)`,

	`CREATE TABLE IF NOT EXISTS mpls_te_tunnels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		tunnel_name TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT '',
		destination TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT '',
		signaled_bw TEXT NOT NULL DEFAULT '',
		explicit_path TEXT NOT NULL DEFAULT '[]',
		actual_path TEXT NOT NULL DEFAULT '[]',
		binding_sid INTEGER NOT NULL DEFAULT 0,
		snapshot_id INTEGER NOT NULL REFERENCES snapshots(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_tunnels_device ON mpls_te_tunnels(device_id)`,

	`CREATE TABLE IF NOT EXISTS sr_mappings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL,
		prefix TEXT NOT NULL,
		sid_index INTEGER NOT NULL DEFAULT 0,
		sid_label INTEGER NOT NULL DEFAULT 0,
		algorithm INTEGER NOT NULL DEFAULT 0,
		flags TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT '',
		snapshot_id INTEGER NOT NULL REFERENCES snapshots(id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_sr_device ON sr_mappings(device_id)`,

	`CREATE TABLE IF NOT EXISTS config_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL REFERENCES devices(id),
		config_text TEXT NOT NULL DEFAULT '',
		diff_from_prev TEXT NOT NULL DEFAULT '',
		captured_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		source_file TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_config_device ON config_snapshots(device_id)`,

	`CREATE TABLE IF NOT EXISTS troubleshoot_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL DEFAULT '',
		symptom TEXT NOT NULL DEFAULT '',
		commands_used TEXT NOT NULL DEFAULT '',
		findings TEXT NOT NULL DEFAULT '',
		resolution TEXT NOT NULL DEFAULT '',
		tags TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	`CREATE TABLE IF NOT EXISTS command_references (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		vendor TEXT NOT NULL DEFAULT '',
		command TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		example_output TEXT NOT NULL DEFAULT '',
		parse_hint TEXT NOT NULL DEFAULT '',
		source_url TEXT NOT NULL DEFAULT ''
	)`,

	`CREATE TABLE IF NOT EXISTS log_ingestions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		file_path TEXT NOT NULL UNIQUE,
		file_hash TEXT NOT NULL DEFAULT '',
		last_offset INTEGER NOT NULL DEFAULT 0,
		processed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	`CREATE TABLE IF NOT EXISTS embedding_meta (
		rowid INTEGER PRIMARY KEY AUTOINCREMENT,
		source_type TEXT NOT NULL DEFAULT '',
		source_id INTEGER NOT NULL DEFAULT 0,
		chunk_text TEXT NOT NULL DEFAULT '',
		model_name TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,

	// FTS5 full-text search indexes
	`CREATE VIRTUAL TABLE IF NOT EXISTS fts_config USING fts5(
		device_id, config_text, source_file,
		content=config_snapshots, content_rowid=id
	)`,

	`CREATE VIRTUAL TABLE IF NOT EXISTS fts_troubleshoot USING fts5(
		symptom, commands_used, findings, resolution, tags,
		content=troubleshoot_logs, content_rowid=id
	)`,

	`CREATE VIRTUAL TABLE IF NOT EXISTS fts_commands USING fts5(
		vendor, command, description,
		content=command_references, content_rowid=id
	)`,

	// Scratch pad: temporary storage for large outputs (full routing tables,
	// specific object queries). FIFO eviction by row limit.
	`CREATE TABLE IF NOT EXISTS scratch_entries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		device_id TEXT NOT NULL DEFAULT '',
		category TEXT NOT NULL DEFAULT 'raw',
		query TEXT NOT NULL DEFAULT '',
		content TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_scratch_device ON scratch_entries(device_id)`,
	`CREATE INDEX IF NOT EXISTS idx_scratch_category ON scratch_entries(category)`,
}
