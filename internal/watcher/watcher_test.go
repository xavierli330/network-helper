// internal/watcher/watcher_test.go
package watcher

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcherDetectsNewFile(t *testing.T) {
	dir := t.TempDir()
	var callCount atomic.Int32

	w, err := New(Config{
		Dirs:         []string{dir},
		Debounce:     100 * time.Millisecond,
		OnFileChange: func(path string) { callCount.Add(1) },
	})
	if err != nil { t.Fatalf("new watcher: %v", err) }

	go w.Start()
	defer w.Stop()

	// Wait for watcher to be ready
	time.Sleep(200 * time.Millisecond)

	// Create a file
	os.WriteFile(filepath.Join(dir, "test.log"), []byte("hello"), 0644)
	time.Sleep(500 * time.Millisecond)

	if callCount.Load() < 1 {
		t.Error("expected at least 1 callback")
	}
}

func TestWatcherDebounce(t *testing.T) {
	dir := t.TempDir()
	var callCount atomic.Int32

	w, err := New(Config{
		Dirs:         []string{dir},
		Debounce:     300 * time.Millisecond,
		OnFileChange: func(path string) { callCount.Add(1) },
	})
	if err != nil { t.Fatalf("new watcher: %v", err) }

	go w.Start()
	defer w.Stop()

	time.Sleep(200 * time.Millisecond)

	// Write multiple times in quick succession
	f := filepath.Join(dir, "test.log")
	for i := 0; i < 5; i++ {
		os.WriteFile(f, []byte("update"), 0644)
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(600 * time.Millisecond)

	// Debounce should collapse to ~1 callback
	count := callCount.Load()
	if count > 2 {
		t.Errorf("expected <=2 callbacks (debounced), got %d", count)
	}
}

func TestWatcherStopClean(t *testing.T) {
	dir := t.TempDir()
	w, err := New(Config{
		Dirs:         []string{dir},
		Debounce:     100 * time.Millisecond,
		OnFileChange: func(path string) {},
	})
	if err != nil { t.Fatalf("new watcher: %v", err) }

	go w.Start()
	time.Sleep(100 * time.Millisecond)
	w.Stop()
	// Should not panic or hang
}
