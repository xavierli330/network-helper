package store

import (
	"strings"
	"sync"
)

// VendorHint represents a row in the vendor_hostname_hints table.
type VendorHint struct {
	ID      int    `json:"id"`
	Keyword string `json:"keyword"`
	Vendor  string `json:"vendor"`
	Note    string `json:"note"`
}

// VendorHintCache provides O(1) hostname→vendor lookups from an in-memory
// copy of the vendor_hostname_hints table. Thread-safe via RWMutex.
type VendorHintCache struct {
	mu    sync.RWMutex
	hints []VendorHint // ordered by keyword length DESC for longest-match
}

// NewVendorHintCache creates an empty cache. Call Reload to populate.
func NewVendorHintCache() *VendorHintCache {
	return &VendorHintCache{}
}

// Reload loads all hints from DB into memory, sorted by keyword length
// descending so that longest-match wins during lookup.
func (c *VendorHintCache) Reload(db *DB) error {
	hints, err := db.ListVendorHints()
	if err != nil {
		return err
	}
	// Sort by keyword length descending (longest match first)
	sortByLenDesc(hints)
	c.mu.Lock()
	c.hints = hints
	c.mu.Unlock()
	return nil
}

// Lookup checks the hostname against all hint keywords.
// Returns (vendor, true) if a keyword is found in the hostname (case-insensitive).
func (c *VendorHintCache) Lookup(hostname string) (string, bool) {
	lower := strings.ToLower(hostname)
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, h := range c.hints {
		if strings.Contains(lower, strings.ToLower(h.Keyword)) {
			return h.Vendor, true
		}
	}
	return "", false
}

// sortByLenDesc sorts hints by keyword length descending.
func sortByLenDesc(hints []VendorHint) {
	// Simple insertion sort — hint count is small (< 100)
	for i := 1; i < len(hints); i++ {
		for j := i; j > 0 && len(hints[j].Keyword) > len(hints[j-1].Keyword); j-- {
			hints[j], hints[j-1] = hints[j-1], hints[j]
		}
	}
}

// ── Store CRUD Methods ───────────────────────────────────────────────────

// ListVendorHints returns all vendor hostname hints ordered by keyword.
func (db *DB) ListVendorHints() ([]VendorHint, error) {
	rows, err := db.Query(`SELECT id, keyword, vendor, note FROM vendor_hostname_hints ORDER BY keyword`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VendorHint
	for rows.Next() {
		var h VendorHint
		if err := rows.Scan(&h.ID, &h.Keyword, &h.Vendor, &h.Note); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// CreateVendorHint inserts a new hint. Returns the ID.
func (db *DB) CreateVendorHint(h VendorHint) (int, error) {
	res, err := db.Exec(`INSERT INTO vendor_hostname_hints (keyword, vendor, note) VALUES (?, ?, ?)`,
		h.Keyword, h.Vendor, h.Note)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

// UpdateVendorHint updates keyword, vendor, and note for an existing hint.
func (db *DB) UpdateVendorHint(h VendorHint) error {
	_, err := db.Exec(`UPDATE vendor_hostname_hints SET keyword=?, vendor=?, note=? WHERE id=?`,
		h.Keyword, h.Vendor, h.Note, h.ID)
	return err
}

// DeleteVendorHint removes a hint by ID.
func (db *DB) DeleteVendorHint(id int) error {
	_, err := db.Exec(`DELETE FROM vendor_hostname_hints WHERE id=?`, id)
	return err
}
