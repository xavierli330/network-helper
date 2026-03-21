package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg.DBPath == "" {
		t.Error("DBPath should have a default")
	}
	if cfg.DataDir == "" {
		t.Error("DataDir should have a default")
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(cfgPath, []byte(`
db_path: /tmp/test.db
watch_dirs:
  - /tmp/logs
`), 0644)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBPath != "/tmp/test.db" {
		t.Errorf("expected /tmp/test.db, got %s", cfg.DBPath)
	}
	if len(cfg.WatchDirs) != 1 || cfg.WatchDirs[0] != "/tmp/logs" {
		t.Errorf("unexpected watch_dirs: %v", cfg.WatchDirs)
	}
}

func TestLoadMissingFileReturnsDefault(t *testing.T) {
	cfg, err := LoadFrom("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("should not error for missing file: %v", err)
	}
	if cfg.DBPath == "" {
		t.Error("should return default config")
	}
}
