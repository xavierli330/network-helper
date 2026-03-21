// internal/watcher/daemon_test.go
package watcher

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadPID(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "test.pid")

	if err := WritePID(pidFile); err != nil {
		t.Fatalf("write: %v", err)
	}

	pid, err := ReadPID(pidFile)
	if err != nil { t.Fatalf("read: %v", err) }
	if pid != os.Getpid() { t.Errorf("expected %d, got %d", os.Getpid(), pid) }
}

func TestReadPIDMissing(t *testing.T) {
	_, err := ReadPID("/nonexistent/pid")
	if err == nil { t.Error("expected error") }
}

func TestRemovePID(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "test.pid")
	WritePID(pidFile)
	RemovePID(pidFile)
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("pid file should be removed")
	}
}

func TestIsRunningCurrentProcess(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "test.pid")
	WritePID(pidFile)
	if !IsRunning(pidFile) { t.Error("current process should be running") }
}

func TestIsRunningStaleProcess(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "test.pid")
	// Write a PID that likely doesn't exist
	os.WriteFile(pidFile, []byte("999999999"), 0644)
	if IsRunning(pidFile) { t.Error("stale PID should not be running") }
}
