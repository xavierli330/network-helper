package store

import (
	"database/sql"
	"fmt"
	"github.com/xavierli/nethelper/internal/model"
)

func (db *DB) UpsertIngestion(ing model.LogIngestion) error {
	_, err := db.Exec(`
		INSERT INTO log_ingestions (file_path, file_hash, last_offset, processed_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(file_path) DO UPDATE SET
			file_hash=excluded.file_hash, last_offset=excluded.last_offset, processed_at=excluded.processed_at`,
		ing.FilePath, ing.FileHash, ing.LastOffset, ing.ProcessedAt)
	return err
}

func (db *DB) GetIngestion(filePath string) (model.LogIngestion, error) {
	var ing model.LogIngestion
	err := db.QueryRow(`SELECT id, file_path, file_hash, last_offset, processed_at FROM log_ingestions WHERE file_path = ?`, filePath).
		Scan(&ing.ID, &ing.FilePath, &ing.FileHash, &ing.LastOffset, &ing.ProcessedAt)
	if err == sql.ErrNoRows {
		return ing, fmt.Errorf("ingestion record for %q not found", filePath)
	}
	return ing, err
}
