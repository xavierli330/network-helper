package store

import (
	"strings"
	"sync"
)

// ClassificationPattern represents a row in the classification_patterns table.
type ClassificationPattern struct {
	ID       int    `json:"id"`
	Vendor   string `json:"vendor"`
	Prefix   string `json:"prefix"`
	CmdType  string `json:"cmd_type"`
	Priority int    `json:"priority"`
}

// PatternCache provides fast prefix→cmdType lookups from an in-memory copy
// of the classification_patterns table. Thread-safe via RWMutex.
type PatternCache struct {
	mu       sync.RWMutex
	patterns map[string][]ClassificationPattern // vendor → patterns sorted by priority DESC, prefix length DESC
}

// NewPatternCache creates an empty cache. Call Reload to populate.
func NewPatternCache() *PatternCache {
	return &PatternCache{patterns: make(map[string][]ClassificationPattern)}
}

// Reload loads all patterns from DB into memory, grouped by vendor and
// sorted by priority DESC then prefix length DESC for longest-match.
func (c *PatternCache) Reload(db *DB) error {
	patterns, err := db.ListClassificationPatterns("")
	if err != nil {
		return err
	}
	grouped := make(map[string][]ClassificationPattern)
	for _, p := range patterns {
		grouped[p.Vendor] = append(grouped[p.Vendor], p)
	}
	// Sort each vendor's patterns: higher priority first, then longer prefix first
	for v := range grouped {
		sortPatterns(grouped[v])
	}
	c.mu.Lock()
	c.patterns = grouped
	c.mu.Unlock()
	return nil
}

// Classify returns the cmdType for a given vendor and command string.
// Matches the longest matching prefix with highest priority.
// Returns ("unknown", false) if no match.
func (c *PatternCache) Classify(vendor, command string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(command))
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, p := range c.patterns[vendor] {
		if strings.HasPrefix(lower, strings.ToLower(p.Prefix)) {
			return p.CmdType, true
		}
	}
	return "unknown", false
}

// AllPatterns returns a snapshot grouped by vendor.
func (c *PatternCache) AllPatterns() map[string][]ClassificationPattern {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string][]ClassificationPattern, len(c.patterns))
	for v, ps := range c.patterns {
		cp := make([]ClassificationPattern, len(ps))
		copy(cp, ps)
		result[v] = cp
	}
	return result
}

// Vendors returns a sorted list of vendor names that have patterns.
func (c *PatternCache) Vendors() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var vs []string
	for v := range c.patterns {
		vs = append(vs, v)
	}
	// Simple sort
	for i := 0; i < len(vs); i++ {
		for j := i + 1; j < len(vs); j++ {
			if vs[j] < vs[i] {
				vs[i], vs[j] = vs[j], vs[i]
			}
		}
	}
	return vs
}

// sortPatterns sorts by priority DESC, then prefix length DESC.
func sortPatterns(ps []ClassificationPattern) {
	for i := 1; i < len(ps); i++ {
		for j := i; j > 0; j-- {
			if ps[j].Priority > ps[j-1].Priority ||
				(ps[j].Priority == ps[j-1].Priority && len(ps[j].Prefix) > len(ps[j-1].Prefix)) {
				ps[j], ps[j-1] = ps[j-1], ps[j]
			} else {
				break
			}
		}
	}
}

// ── Store CRUD Methods ───────────────────────────────────────────────────

// ListClassificationPatterns returns patterns filtered by vendor (empty = all).
func (db *DB) ListClassificationPatterns(vendor string) ([]ClassificationPattern, error) {
	q := `SELECT id, vendor, prefix, cmd_type, priority FROM classification_patterns`
	var args []any
	if vendor != "" {
		q += ` WHERE vendor = ?`
		args = append(args, vendor)
	}
	q += ` ORDER BY vendor, priority DESC, length(prefix) DESC`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClassificationPattern
	for rows.Next() {
		var p ClassificationPattern
		if err := rows.Scan(&p.ID, &p.Vendor, &p.Prefix, &p.CmdType, &p.Priority); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CreateClassificationPattern inserts a new pattern. Returns the ID.
func (db *DB) CreateClassificationPattern(p ClassificationPattern) (int, error) {
	res, err := db.Exec(`INSERT INTO classification_patterns (vendor, prefix, cmd_type, priority) VALUES (?, ?, ?, ?)`,
		p.Vendor, p.Prefix, p.CmdType, p.Priority)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

// UpdateClassificationPattern updates an existing pattern.
func (db *DB) UpdateClassificationPattern(p ClassificationPattern) error {
	_, err := db.Exec(`UPDATE classification_patterns SET vendor=?, prefix=?, cmd_type=?, priority=? WHERE id=?`,
		p.Vendor, p.Prefix, p.CmdType, p.Priority, p.ID)
	return err
}

// DeleteClassificationPattern removes a pattern by ID.
func (db *DB) DeleteClassificationPattern(id int) error {
	_, err := db.Exec(`DELETE FROM classification_patterns WHERE id=?`, id)
	return err
}

// SeedClassificationPatterns inserts the default patterns if the table is empty.
// This migrates the 55 hardcoded rules from Go code to DB.
func (db *DB) SeedClassificationPatterns() error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM classification_patterns`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil // already seeded
	}

	seeds := []ClassificationPattern{
		// ── Huawei ──
		{Vendor: "huawei", Prefix: "display ip routing-table", CmdType: "rib", Priority: 10},
		{Vendor: "huawei", Prefix: "display ip rout", CmdType: "rib", Priority: 5},
		{Vendor: "huawei", Prefix: "display fib", CmdType: "fib", Priority: 10},
		{Vendor: "huawei", Prefix: "display mpls lsp", CmdType: "lfib", Priority: 10},
		{Vendor: "huawei", Prefix: "display mpls forwarding", CmdType: "lfib", Priority: 10},
		{Vendor: "huawei", Prefix: "display interface", CmdType: "interface", Priority: 10},
		{Vendor: "huawei", Prefix: "display int", CmdType: "interface", Priority: 5},
		{Vendor: "huawei", Prefix: "display ospf peer", CmdType: "neighbor", Priority: 10},
		{Vendor: "huawei", Prefix: "display bgp peer", CmdType: "neighbor", Priority: 10},
		{Vendor: "huawei", Prefix: "display isis peer", CmdType: "neighbor", Priority: 10},
		{Vendor: "huawei", Prefix: "display mpls ldp session", CmdType: "neighbor", Priority: 10},
		{Vendor: "huawei", Prefix: "display mpls ldp peer", CmdType: "neighbor", Priority: 10},
		{Vendor: "huawei", Prefix: "display rsvp session", CmdType: "neighbor", Priority: 10},
		{Vendor: "huawei", Prefix: "display lldp neighbor", CmdType: "neighbor", Priority: 10},
		{Vendor: "huawei", Prefix: "display mpls te tunnel", CmdType: "tunnel", Priority: 10},
		{Vendor: "huawei", Prefix: "display segment-routing", CmdType: "sr_mapping", Priority: 10},
		{Vendor: "huawei", Prefix: "display isis segment-routing", CmdType: "sr_mapping", Priority: 10},
		{Vendor: "huawei", Prefix: "display current-configuration", CmdType: "config", Priority: 10},
		{Vendor: "huawei", Prefix: "display saved-configuration", CmdType: "config", Priority: 10},
		{Vendor: "huawei", Prefix: "display cur", CmdType: "config", Priority: 5},
		{Vendor: "huawei", Prefix: "display sa", CmdType: "config", Priority: 5},
		// ── Cisco ──
		{Vendor: "cisco", Prefix: "show ip route", CmdType: "rib", Priority: 10},
		{Vendor: "cisco", Prefix: "show route", CmdType: "rib", Priority: 5},
		{Vendor: "cisco", Prefix: "show ip cef", CmdType: "fib", Priority: 10},
		{Vendor: "cisco", Prefix: "show mpls forwarding", CmdType: "lfib", Priority: 10},
		{Vendor: "cisco", Prefix: "show interface", CmdType: "interface", Priority: 10},
		{Vendor: "cisco", Prefix: "show ip interface", CmdType: "interface", Priority: 10},
		{Vendor: "cisco", Prefix: "show ip ospf neighbor", CmdType: "neighbor", Priority: 10},
		{Vendor: "cisco", Prefix: "show ip bgp summary", CmdType: "neighbor", Priority: 10},
		{Vendor: "cisco", Prefix: "show bgp summary", CmdType: "neighbor", Priority: 10},
		{Vendor: "cisco", Prefix: "show isis neighbor", CmdType: "neighbor", Priority: 10},
		{Vendor: "cisco", Prefix: "show mpls ldp neighbor", CmdType: "neighbor", Priority: 10},
		{Vendor: "cisco", Prefix: "show lldp neighbor", CmdType: "neighbor", Priority: 10},
		{Vendor: "cisco", Prefix: "show mpls traffic-eng tunnel", CmdType: "tunnel", Priority: 10},
		{Vendor: "cisco", Prefix: "show running-config", CmdType: "config", Priority: 10},
		{Vendor: "cisco", Prefix: "show startup-config", CmdType: "config", Priority: 10},
		// ── H3C ──
		{Vendor: "h3c", Prefix: "display ip routing-table", CmdType: "rib", Priority: 10},
		{Vendor: "h3c", Prefix: "display fib", CmdType: "fib", Priority: 10},
		{Vendor: "h3c", Prefix: "display mpls lsp", CmdType: "lfib", Priority: 10},
		{Vendor: "h3c", Prefix: "display mpls forwarding", CmdType: "lfib", Priority: 10},
		{Vendor: "h3c", Prefix: "display ip interface", CmdType: "interface", Priority: 10},
		{Vendor: "h3c", Prefix: "display interface", CmdType: "interface", Priority: 5},
		{Vendor: "h3c", Prefix: "display ospf peer", CmdType: "neighbor", Priority: 10},
		{Vendor: "h3c", Prefix: "display bgp peer", CmdType: "neighbor", Priority: 10},
		{Vendor: "h3c", Prefix: "display isis peer", CmdType: "neighbor", Priority: 10},
		{Vendor: "h3c", Prefix: "display mpls ldp session", CmdType: "neighbor", Priority: 10},
		{Vendor: "h3c", Prefix: "display current-configuration", CmdType: "config", Priority: 10},
		// ── Juniper ──
		{Vendor: "juniper", Prefix: "show route", CmdType: "rib", Priority: 10},
		{Vendor: "juniper", Prefix: "show interface", CmdType: "interface", Priority: 10},
		{Vendor: "juniper", Prefix: "show ospf neighbor", CmdType: "neighbor", Priority: 10},
		{Vendor: "juniper", Prefix: "show bgp summary", CmdType: "neighbor", Priority: 10},
		{Vendor: "juniper", Prefix: "show isis adjacency", CmdType: "neighbor", Priority: 10},
		{Vendor: "juniper", Prefix: "show ldp session", CmdType: "neighbor", Priority: 10},
		{Vendor: "juniper", Prefix: "show rsvp session", CmdType: "tunnel", Priority: 10},
		{Vendor: "juniper", Prefix: "show route table mpls", CmdType: "lfib", Priority: 10},
		{Vendor: "juniper", Prefix: "show configuration", CmdType: "config", Priority: 10},
		{Vendor: "juniper", Prefix: "| display set", CmdType: "config_set", Priority: 10},
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO classification_patterns (vendor, prefix, cmd_type, priority) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, s := range seeds {
		if _, err := stmt.Exec(s.Vendor, s.Prefix, s.CmdType, s.Priority); err != nil {
			return err
		}
	}
	return tx.Commit()
}
