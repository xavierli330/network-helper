# Nethelper Plan 5: Knowledge System + FTS5 Search

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add FTS5 full-text search indexes, config diff, troubleshoot notes CRUD, and the `search`, `diff`, and `note` CLI commands.

**Architecture:** FTS5 virtual tables are added via new migrations for config_snapshots, troubleshoot_logs, and command_references. A new `search` store layer provides unified search across all indexed content. The `diff` feature uses Go's standard text diff to compare config snapshots or route tables between time points. The `note` commands provide manual CRUD for troubleshoot_logs. All features are wired to CLI subcommands.

**Tech Stack:** Go 1.24+, SQLite FTS5 (built into ncruces/go-sqlite3), existing store/model packages

**Spec:** `docs/superpowers/specs/2026-03-21-network-helper-design.md` (Sections 5, 9)

**Depends on:** Plan 1 (store with tables), Plan 2-3 (parsers that produce config data)

---

## File Structure

```
internal/
├── store/
│   ├── migrations.go              # Modify: add FTS5 virtual tables
│   ├── search_store.go            # New: FTS5 search across config/logs/commands
│   ├── search_store_test.go       # Tests
│   ├── troubleshoot_store.go      # New: CRUD for troubleshoot_logs
│   └── troubleshoot_store_test.go # Tests
├── diff/
│   ├── diff.go                    # Text diff engine (unified diff format)
│   └── diff_test.go               # Tests
├── cli/
│   ├── search.go                  # New: search config/log/command
│   ├── diff.go                    # New: diff config/route/snapshot
│   ├── note.go                    # New: note add/list/search
│   └── root.go                    # Modify: register search/diff/note
```

---

### Task 1: FTS5 Migrations

**Files:**
- Modify: `internal/store/migrations.go`

- [ ] **Step 1: Add FTS5 virtual table migrations**

Append to the `migrations` slice in `internal/store/migrations.go`:

```go
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
```

- [ ] **Step 2: Write test to verify FTS5 tables exist**

Add to `internal/store/db_test.go`:

```go
func TestFTS5TablesExist(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil { t.Fatalf("open: %v", err) }
	defer db.Close()

	ftsTables := []string{"fts_config", "fts_troubleshoot", "fts_commands"}
	for _, table := range ftsTables {
		var count int
		// FTS5 tables support SELECT
		err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("FTS5 table %s does not exist or is broken: %v", table, err)
		}
	}
}
```

- [ ] **Step 3: Run test — verify PASS**

Run: `go test ./internal/store/ -v -run TestFTS5`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/store/migrations.go internal/store/db_test.go
git commit -m "feat: add FTS5 virtual tables for config, troubleshoot, and commands"
```

---

### Task 2: Troubleshoot Store — CRUD + FTS Sync

**Files:**
- Create: `internal/store/troubleshoot_store.go`
- Create: `internal/store/troubleshoot_store_test.go`

- [ ] **Step 1: Write test**

```go
// internal/store/troubleshoot_store_test.go
package store

import (
	"testing"
	"github.com/xavierli/nethelper/internal/model"
)

func TestInsertAndListTroubleshootLogs(t *testing.T) {
	db := testDB(t)

	log1 := model.TroubleshootLog{
		Symptom: "OSPF neighbor flapping", Findings: "MTU mismatch on GE0/0/1",
		Resolution: "Set MTU to 1500 on both ends", Tags: "ospf,mtu,flap",
	}
	id, err := db.InsertTroubleshootLog(log1)
	if err != nil { t.Fatalf("insert: %v", err) }
	if id == 0 { t.Error("expected non-zero ID") }

	log2 := model.TroubleshootLog{
		Symptom: "BGP session down", Findings: "ACL blocking TCP 179",
		Resolution: "Updated ACL to permit BGP", Tags: "bgp,acl",
	}
	db.InsertTroubleshootLog(log2)

	logs, err := db.ListTroubleshootLogs(10, 0)
	if err != nil { t.Fatalf("list: %v", err) }
	if len(logs) != 2 { t.Fatalf("expected 2, got %d", len(logs)) }
	// Most recent first
	if logs[0].Symptom != "BGP session down" { t.Errorf("order: %s", logs[0].Symptom) }
}

func TestSearchTroubleshootLogs(t *testing.T) {
	db := testDB(t)

	db.InsertTroubleshootLog(model.TroubleshootLog{
		Symptom: "OSPF neighbor flapping", Findings: "MTU mismatch",
		Resolution: "Set MTU to 1500", Tags: "ospf,mtu",
	})
	db.InsertTroubleshootLog(model.TroubleshootLog{
		Symptom: "BGP session down", Findings: "ACL blocking TCP 179",
		Resolution: "Updated ACL", Tags: "bgp,acl",
	})

	// Search for "OSPF"
	results, err := db.SearchTroubleshootLogs("OSPF")
	if err != nil { t.Fatalf("search: %v", err) }
	if len(results) != 1 { t.Fatalf("expected 1 result, got %d", len(results)) }
	if results[0].Symptom != "OSPF neighbor flapping" { t.Errorf("got: %s", results[0].Symptom) }

	// Search for "MTU" (in findings)
	results, _ = db.SearchTroubleshootLogs("MTU")
	if len(results) != 1 { t.Errorf("expected 1, got %d", len(results)) }

	// Search for something not present
	results, _ = db.SearchTroubleshootLogs("ISIS")
	if len(results) != 0 { t.Errorf("expected 0, got %d", len(results)) }
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/store/ -v -run TestInsertAndListTroubleshoot`
Expected: FAIL

- [ ] **Step 3: Implement troubleshoot_store.go**

```go
// internal/store/troubleshoot_store.go
package store

import (
	"github.com/xavierli/nethelper/internal/model"
)

func (db *DB) InsertTroubleshootLog(log model.TroubleshootLog) (int, error) {
	result, err := db.Exec(`INSERT INTO troubleshoot_logs (device_id, symptom, commands_used, findings, resolution, tags)
		VALUES (?, ?, ?, ?, ?, ?)`,
		log.DeviceID, log.Symptom, log.CommandsUsed, log.Findings, log.Resolution, log.Tags)
	if err != nil { return 0, err }

	id, err := result.LastInsertId()
	if err != nil { return 0, err }

	// Sync to FTS5 index
	db.Exec(`INSERT INTO fts_troubleshoot(rowid, symptom, commands_used, findings, resolution, tags)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, log.Symptom, log.CommandsUsed, log.Findings, log.Resolution, log.Tags)

	return int(id), nil
}

func (db *DB) ListTroubleshootLogs(limit, offset int) ([]model.TroubleshootLog, error) {
	rows, err := db.Query(`SELECT id, device_id, symptom, commands_used, findings, resolution, tags, created_at
		FROM troubleshoot_logs ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil { return nil, err }
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
	if err != nil { return nil, err }
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
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/store/ -v -run TestInsertAndListTroubleshoot`
Run: `go test ./internal/store/ -v -run TestSearchTroubleshoot`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/troubleshoot_store.go internal/store/troubleshoot_store_test.go
git commit -m "feat: add troubleshoot log CRUD with FTS5 search"
```

---

### Task 3: Search Store — Unified Config + Command Search

**Files:**
- Create: `internal/store/search_store.go`
- Create: `internal/store/search_store_test.go`

- [ ] **Step 1: Write test**

```go
// internal/store/search_store_test.go
package store

import (
	"testing"
	"time"
	"github.com/xavierli/nethelper/internal/model"
)

func TestSearchConfig(t *testing.T) {
	db := testDB(t)
	seedDevice(t, db)

	db.InsertConfigSnapshot(model.ConfigSnapshot{
		DeviceID: "d1", ConfigText: "interface GE0/0/1\n ip address 10.0.0.1 24\n ospf cost 100\n#",
		SourceFile: "test.log",
	})
	db.InsertConfigSnapshot(model.ConfigSnapshot{
		DeviceID: "d1", ConfigText: "interface GE0/0/2\n ip address 10.0.0.2 24\n#",
		SourceFile: "test2.log",
	})

	// Sync FTS index
	db.SyncConfigFTS()

	results, err := db.SearchConfig("ospf cost")
	if err != nil { t.Fatalf("search: %v", err) }
	if len(results) != 1 { t.Fatalf("expected 1, got %d", len(results)) }
}

func TestSearchCommands(t *testing.T) {
	db := testDB(t)

	db.InsertCommandReference(model.CommandReference{
		Vendor: "huawei", Command: "display ip routing-table",
		Description: "Show the IP routing table with all routes and their attributes",
	})
	db.InsertCommandReference(model.CommandReference{
		Vendor: "huawei", Command: "display mpls ldp session",
		Description: "Show MPLS LDP session status and peer information",
	})

	results, err := db.SearchCommands("routing table")
	if err != nil { t.Fatalf("search: %v", err) }
	if len(results) != 1 { t.Fatalf("expected 1, got %d", len(results)) }
	if results[0].Command != "display ip routing-table" { t.Errorf("got: %s", results[0].Command) }
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/store/ -v -run TestSearchConfig`
Expected: FAIL

- [ ] **Step 3: Implement search_store.go**

```go
// internal/store/search_store.go
package store

import (
	"database/sql"
	"github.com/xavierli/nethelper/internal/model"
)

// SyncConfigFTS rebuilds the FTS index for config_snapshots.
func (db *DB) SyncConfigFTS() error {
	// Clear and rebuild
	db.Exec(`DELETE FROM fts_config`)
	_, err := db.Exec(`INSERT INTO fts_config(rowid, device_id, config_text, source_file)
		SELECT id, device_id, config_text, source_file FROM config_snapshots`)
	return err
}

// SearchConfig searches config snapshots using FTS5.
func (db *DB) SearchConfig(query string) ([]model.ConfigSnapshot, error) {
	rows, err := db.Query(`SELECT c.id, c.device_id, c.config_text, c.diff_from_prev, c.captured_at, c.source_file
		FROM config_snapshots c
		JOIN fts_config f ON c.id = f.rowid
		WHERE fts_config MATCH ?
		ORDER BY rank`, query)
	if err != nil { return nil, err }
	defer rows.Close()

	var results []model.ConfigSnapshot
	for rows.Next() {
		var cs model.ConfigSnapshot
		if err := rows.Scan(&cs.ID, &cs.DeviceID, &cs.ConfigText, &cs.DiffFromPrev, &cs.CapturedAt, &cs.SourceFile); err != nil {
			return nil, err
		}
		results = append(results, cs)
	}
	return results, rows.Err()
}

// InsertCommandReference inserts a command reference and syncs FTS.
func (db *DB) InsertCommandReference(ref model.CommandReference) (int, error) {
	result, err := db.Exec(`INSERT INTO command_references (vendor, command, description, example_output, parse_hint, source_url)
		VALUES (?, ?, ?, ?, ?, ?)`,
		ref.Vendor, ref.Command, ref.Description, ref.ExampleOutput, ref.ParseHint, ref.SourceURL)
	if err != nil { return 0, err }

	id, err := result.LastInsertId()
	if err != nil { return 0, err }

	db.Exec(`INSERT INTO fts_commands(rowid, vendor, command, description) VALUES (?, ?, ?, ?)`,
		id, ref.Vendor, ref.Command, ref.Description)

	return int(id), nil
}

// SearchCommands searches command references using FTS5.
func (db *DB) SearchCommands(query string) ([]model.CommandReference, error) {
	rows, err := db.Query(`SELECT c.id, c.vendor, c.command, c.description, c.example_output, c.parse_hint, c.source_url
		FROM command_references c
		JOIN fts_commands f ON c.id = f.rowid
		WHERE fts_commands MATCH ?
		ORDER BY rank`, query)
	if err != nil { return nil, err }
	defer rows.Close()

	var results []model.CommandReference
	for rows.Next() {
		var r model.CommandReference
		if err := rows.Scan(&r.ID, &r.Vendor, &r.Command, &r.Description, &r.ExampleOutput, &r.ParseHint, &r.SourceURL); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// ListCommandReferences lists all command references, optionally filtered by vendor.
func (db *DB) ListCommandReferences(vendor string) ([]model.CommandReference, error) {
	query := `SELECT id, vendor, command, description, example_output, parse_hint, source_url FROM command_references`
	var rows *sql.Rows
	var err error
	if vendor != "" {
		rows, err = db.Query(query+" WHERE vendor = ? ORDER BY command", vendor)
	} else {
		rows, err = db.Query(query+" ORDER BY vendor, command")
	}
	if err != nil { return nil, err }
	defer rows.Close()

	var results []model.CommandReference
	for rows.Next() {
		var r model.CommandReference
		if err := rows.Scan(&r.ID, &r.Vendor, &r.Command, &r.Description, &r.ExampleOutput, &r.ParseHint, &r.SourceURL); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/store/ -v -run "TestSearchConfig|TestSearchCommand"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/search_store.go internal/store/search_store_test.go
git commit -m "feat: add FTS5 search for config snapshots and command references"
```

---

### Task 4: Diff Engine

**Files:**
- Create: `internal/diff/diff.go`
- Create: `internal/diff/diff_test.go`

- [ ] **Step 1: Write test**

```go
// internal/diff/diff_test.go
package diff

import (
	"strings"
	"testing"
)

func TestUnifiedDiff(t *testing.T) {
	old := `interface GE0/0/1
 ip address 10.0.0.1 24
 ospf cost 100
#
interface GE0/0/2
 ip address 10.0.0.2 24
#`

	new_ := `interface GE0/0/1
 ip address 10.0.0.1 24
 ospf cost 200
#
interface GE0/0/2
 ip address 10.0.0.2 24
 description To-PE-01
#`

	result := Unified(old, new_, "before", "after")
	if result == "" { t.Fatal("expected non-empty diff") }
	if !strings.Contains(result, "-") && !strings.Contains(result, "+") {
		t.Error("diff should contain - and + lines")
	}
	// The cost change should appear
	if !strings.Contains(result, "cost") { t.Error("diff should mention cost change") }
}

func TestUnifiedDiffIdentical(t *testing.T) {
	text := "line1\nline2\nline3"
	result := Unified(text, text, "a", "b")
	if result != "" { t.Errorf("expected empty diff for identical, got: %s", result) }
}

func TestUnifiedDiffEmpty(t *testing.T) {
	result := Unified("", "new content", "a", "b")
	if result == "" { t.Error("expected diff for empty→content") }
}
```

- [ ] **Step 2: Run test — verify FAIL**

Run: `go test ./internal/diff/ -v`
Expected: FAIL

- [ ] **Step 3: Add go-diff dependency and implement diff.go**

Run: `go get github.com/sergi/go-diff`

```go
// internal/diff/diff.go
package diff

import (
	"fmt"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// Unified produces a unified diff between two texts.
// Returns empty string if texts are identical.
func Unified(oldText, newText, oldName, newName string) string {
	if oldText == newText { return "" }

	dmp := diffmatchpatch.New()
	a, b, lineArray := dmp.DiffLinesToChars(oldText, newText)
	diffs := dmp.DiffMain(a, b, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)
	diffs = dmp.DiffCleanupSemantic(diffs)

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n", oldName)
	fmt.Fprintf(&sb, "+++ %s\n", newName)

	for _, d := range diffs {
		lines := strings.Split(strings.TrimRight(d.Text, "\n"), "\n")
		for _, line := range lines {
			switch d.Type {
			case diffmatchpatch.DiffDelete:
				fmt.Fprintf(&sb, "-%s\n", line)
			case diffmatchpatch.DiffInsert:
				fmt.Fprintf(&sb, "+%s\n", line)
			case diffmatchpatch.DiffEqual:
				fmt.Fprintf(&sb, " %s\n", line)
			}
		}
	}

	return sb.String()
}
```

- [ ] **Step 4: Run test — verify PASS**

Run: `go test ./internal/diff/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/diff/
git commit -m "feat: add unified diff engine with LCS algorithm"
```

---

### Task 5: CLI — note add/list/search

**Files:**
- Create: `internal/cli/note.go`

- [ ] **Step 1: Implement note.go**

```go
// internal/cli/note.go
package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/model"
)

func newNoteCmd() *cobra.Command {
	note := &cobra.Command{
		Use:   "note",
		Short: "Troubleshooting notes",
	}
	note.AddCommand(newNoteAddCmd())
	note.AddCommand(newNoteListCmd())
	note.AddCommand(newNoteSearchCmd())
	return note
}

func newNoteAddCmd() *cobra.Command {
	var symptom, findings, resolution, tags, deviceID, commands string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a troubleshooting note",
		RunE: func(cmd *cobra.Command, args []string) error {
			if symptom == "" { return fmt.Errorf("--symptom is required") }
			log := model.TroubleshootLog{
				DeviceID:     deviceID,
				Symptom:      symptom,
				CommandsUsed: commands,
				Findings:     findings,
				Resolution:   resolution,
				Tags:         tags,
			}
			id, err := db.InsertTroubleshootLog(log)
			if err != nil { return fmt.Errorf("insert: %w", err) }
			fmt.Printf("Note #%d created.\n", id)
			return nil
		},
	}
	cmd.Flags().StringVar(&symptom, "symptom", "", "problem symptom (required)")
	cmd.Flags().StringVar(&findings, "finding", "", "what you found")
	cmd.Flags().StringVar(&resolution, "resolution", "", "how it was resolved")
	cmd.Flags().StringVar(&tags, "tags", "", "comma-separated tags (e.g., ospf,mtu)")
	cmd.Flags().StringVar(&deviceID, "device", "", "related device ID")
	cmd.Flags().StringVar(&commands, "commands", "", "commands used during troubleshooting")
	return cmd
}

func newNoteListCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List troubleshooting notes",
		RunE: func(cmd *cobra.Command, args []string) error {
			logs, err := db.ListTroubleshootLogs(limit, 0)
			if err != nil { return err }
			if len(logs) == 0 {
				fmt.Println("No notes found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "ID\tDATE\tSYMPTOM\tTAGS\n")
			for _, l := range logs {
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", l.ID, l.CreatedAt.Format("2006-01-02"), l.Symptom, l.Tags)
			}
			return w.Flush()
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "max results")
	return cmd
}

func newNoteSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search troubleshooting notes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := db.SearchTroubleshootLogs(args[0])
			if err != nil { return err }
			if len(results) == 0 {
				fmt.Println("No matching notes found.")
				return nil
			}
			for _, l := range results {
				fmt.Printf("--- Note #%d [%s] tags:%s ---\n", l.ID, l.CreatedAt.Format("2006-01-02"), l.Tags)
				fmt.Printf("Symptom:    %s\n", l.Symptom)
				if l.Findings != "" { fmt.Printf("Findings:   %s\n", l.Findings) }
				if l.Resolution != "" { fmt.Printf("Resolution: %s\n", l.Resolution) }
				fmt.Println()
			}
			return nil
		},
	}
}
```

- [ ] **Step 2: Register in root.go**

Add `root.AddCommand(newNoteCmd())` in `NewRootCmd`.

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/nethelper && ./nethelper note --help`
Expected: shows add/list/search

- [ ] **Step 4: Commit**

```bash
git add internal/cli/note.go internal/cli/root.go
git commit -m "feat: add note add/list/search CLI commands"
```

---

### Task 6: CLI — search config/log/command

**Files:**
- Create: `internal/cli/search.go`

- [ ] **Step 1: Implement search.go**

```go
// internal/cli/search.go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	search := &cobra.Command{
		Use:   "search",
		Short: "Full-text search across data",
	}
	search.AddCommand(newSearchConfigCmd())
	search.AddCommand(newSearchLogCmd())
	search.AddCommand(newSearchCommandCmd())
	return search
}

func newSearchConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config <query>",
		Short: "Search configuration content",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db.SyncConfigFTS() // ensure FTS is up to date
			results, err := db.SearchConfig(args[0])
			if err != nil { return err }
			if len(results) == 0 {
				fmt.Println("No matching configs found.")
				return nil
			}
			for _, cs := range results {
				fmt.Printf("--- Device: %s [%s] from %s ---\n", cs.DeviceID, cs.CapturedAt.Format("2006-01-02 15:04"), cs.SourceFile)
				// Show snippet (first 500 chars)
				text := cs.ConfigText
				if len(text) > 500 { text = text[:500] + "..." }
				fmt.Println(text)
				fmt.Println()
			}
			return nil
		},
	}
}

func newSearchLogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "log <query>",
		Short: "Search troubleshooting logs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := db.SearchTroubleshootLogs(args[0])
			if err != nil { return err }
			if len(results) == 0 {
				fmt.Println("No matching logs found.")
				return nil
			}
			for _, l := range results {
				fmt.Printf("--- Note #%d [%s] tags:%s ---\n", l.ID, l.CreatedAt.Format("2006-01-02"), l.Tags)
				fmt.Printf("Symptom:    %s\n", l.Symptom)
				if l.Findings != "" { fmt.Printf("Findings:   %s\n", l.Findings) }
				if l.Resolution != "" { fmt.Printf("Resolution: %s\n", l.Resolution) }
				fmt.Println()
			}
			return nil
		},
	}
}

func newSearchCommandCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "command <query>",
		Short: "Search command references",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			results, err := db.SearchCommands(args[0])
			if err != nil { return err }
			if len(results) == 0 {
				fmt.Println("No matching commands found.")
				return nil
			}
			for _, r := range results {
				fmt.Printf("[%s] %s\n  %s\n\n", r.Vendor, r.Command, r.Description)
			}
			return nil
		},
	}
}
```

- [ ] **Step 2: Register in root.go**

Add `root.AddCommand(newSearchCmd())` in `NewRootCmd`.

- [ ] **Step 3: Commit**

```bash
git add internal/cli/search.go internal/cli/root.go
git commit -m "feat: add search config/log/command CLI commands"
```

---

### Task 7: CLI — diff config/route

**Files:**
- Create: `internal/cli/diff.go`

- [ ] **Step 1: Implement diff.go**

```go
// internal/cli/diff.go
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xavierli/nethelper/internal/diff"
	"github.com/xavierli/nethelper/internal/model"
)

func newDiffCmd() *cobra.Command {
	d := &cobra.Command{
		Use:   "diff",
		Short: "Compare configurations or data between time points",
	}
	d.AddCommand(newDiffConfigCmd())
	d.AddCommand(newDiffRouteCmd())
	return d
}

func newDiffConfigCmd() *cobra.Command {
	var deviceID string
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Compare configuration snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" { return fmt.Errorf("--device is required") }

			snapshots, err := db.GetConfigSnapshots(deviceID)
			if err != nil { return err }
			if len(snapshots) < 2 {
				fmt.Println("Need at least 2 config snapshots to diff.")
				return nil
			}

			newer := snapshots[0]
			older := snapshots[1]

			result := diff.Unified(older.ConfigText, newer.ConfigText,
				fmt.Sprintf("%s [%s]", deviceID, older.CapturedAt.Format("2006-01-02 15:04")),
				fmt.Sprintf("%s [%s]", deviceID, newer.CapturedAt.Format("2006-01-02 15:04")))

			if result == "" {
				fmt.Println("No configuration changes between the two snapshots.")
				return nil
			}
			fmt.Println(result)
			return nil
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	return cmd
}

func newDiffRouteCmd() *cobra.Command {
	var deviceID string
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Compare routing tables between snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			if deviceID == "" { return fmt.Errorf("--device is required") }

			snapIDs, err := db.GetRIBSnapshotIDs(deviceID, 2)
			if err != nil || len(snapIDs) < 2 {
				fmt.Println("Need at least 2 route snapshots to diff.")
				return nil
			}

			newer, _ := db.GetRIBEntries(deviceID, snapIDs[0])
			older, _ := db.GetRIBEntries(deviceID, snapIDs[1])

			oldText := routeEntriesToText(older)
			newText := routeEntriesToText(newer)

			result := diff.Unified(oldText, newText,
				fmt.Sprintf("snapshot-%d", snapIDs[1]),
				fmt.Sprintf("snapshot-%d", snapIDs[0]))

			if result == "" {
				fmt.Println("No routing table changes between the two snapshots.")
				return nil
			}
			fmt.Println(result)
			return nil
		},
	}
	cmd.Flags().StringVar(&deviceID, "device", "", "device ID")
	return cmd
}

func routeEntriesToText(entries []model.RIBEntry) string {
	var lines []string
	for _, e := range entries {
		lines = append(lines, fmt.Sprintf("%-20s %-8s %-4d %-6d %-16s %s",
			fmt.Sprintf("%s/%d", e.Prefix, e.MaskLen), e.Protocol, e.Preference, e.Metric, e.NextHop, e.OutgoingInterface))
	}
	return strings.Join(lines, "\n")
}
```

Note: Also add `GetRIBSnapshotIDs` to the store. Append this to `internal/store/rib_store.go`:

```go
// GetRIBSnapshotIDs returns the most recent snapshot IDs that contain RIB data for a device.
func (db *DB) GetRIBSnapshotIDs(deviceID string, limit int) ([]int, error) {
	rows, err := db.Query(`SELECT DISTINCT snapshot_id FROM rib_entries WHERE device_id = ? ORDER BY snapshot_id DESC LIMIT ?`, deviceID, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
```

- [ ] **Step 2: Register in root.go**

Add `root.AddCommand(newDiffCmd())` in `NewRootCmd`.

- [ ] **Step 3: Build and verify**

Run: `go build ./cmd/nethelper`
Run: `./nethelper diff --help` → shows config/route
Run: `./nethelper search --help` → shows config/log/command
Run: `./nethelper note --help` → shows add/list/search

- [ ] **Step 4: Run full test suite**

Run: `go test ./... -timeout 60s`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cli/diff.go internal/cli/root.go
git commit -m "feat: add diff config/route CLI commands with unified diff output"
```

- [ ] **Step 6: Clean up**

Run: `rm -f nethelper`
