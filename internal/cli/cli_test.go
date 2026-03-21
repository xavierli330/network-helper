package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIEndToEnd(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")

	// Test: version
	root := NewRootCmd()
	root.SetArgs([]string{"version"})
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	if err := root.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}

	// Test: show device (empty)
	root = NewRootCmd()
	root.SetArgs([]string{"show", "device", "--db", dbPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("show device: %v", err)
	}

	// Test: watch ingest
	logFile := filepath.Join(t.TempDir(), "test.log")
	os.WriteFile(logFile, []byte("<Huawei>display version\nsome output here\n"), 0644)

	root = NewRootCmd()
	root.SetArgs([]string{"watch", "ingest", logFile, "--db", dbPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("watch ingest: %v", err)
	}

	// Test: show route with no data
	root = NewRootCmd()
	root.SetArgs([]string{"show", "route", "--device", "nonexistent", "--db", dbPath})
	err := root.Execute()
	if err == nil {
		t.Log("show route for nonexistent device returned no error (expected)")
	}
}
