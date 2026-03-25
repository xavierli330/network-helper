package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

type DB struct {
	*sql.DB
	path string
}

func Open(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}
	sqlDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA journal_mode=WAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA foreign_keys=ON"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set synchronous mode: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA cache_size=-64000"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set cache size: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA temp_store=MEMORY"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set temp store: %w", err)
	}
	db := &DB{DB: sqlDB, path: dbPath}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return db, nil
}

func (db *DB) migrate() error {
	for i, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			// ALTER TABLE ... ADD COLUMN is not idempotent in SQLite.
			// Tolerate "duplicate column name" errors so that the migration
			// runner is safe to run on an already-migrated database.
			isAddColumn := strings.Contains(strings.ToUpper(m), "ADD COLUMN")
			isDuplicate := strings.Contains(err.Error(), "duplicate column name")
			if isAddColumn && isDuplicate {
				continue
			}
			return fmt.Errorf("migration %d failed: %w", i, err)
		}
	}
	return nil
}

func (db *DB) Path() string {
	return db.path
}
