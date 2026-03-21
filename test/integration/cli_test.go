//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binaryPath holds the path to the compiled nethelper binary built in TestMain.
// This file defines TestMain; helpers_test.go does not, so there is no conflict.
var binaryPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "nethelper-cli-test-*")
	if err != nil {
		panic("MkdirTemp: " + err.Error())
	}

	binaryPath = filepath.Join(tmpDir, "nethelper")

	cmd := exec.Command("go", "build", "-o", binaryPath, "./cmd/nethelper")
	cmd.Dir = findProjectRoot()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("build failed: " + err.Error())
	}

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// findProjectRoot walks up from the current working directory until it finds
// a go.mod file, which marks the repository root.
func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		panic("Getwd: " + err.Error())
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("could not find project root (no go.mod found)")
		}
		dir = parent
	}
}

// runNethelper executes the compiled binary with --db <dbPath> prepended to
// args and returns the combined stdout+stderr output together with any error.
func runNethelper(t *testing.T, dbPath string, args ...string) (string, error) {
	t.Helper()
	fullArgs := append([]string{"--db", dbPath}, args...)
	cmd := exec.Command(binaryPath, fullArgs...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// testdataPath returns the absolute path to a file under testdata/.
// The path must be relative to the testdata directory (e.g. "huawei/foo.log").
func testdataPath(relPath string) string {
	// test files live next to the testdata directory, so simply join.
	return filepath.Join("testdata", relPath)
}

// ──────────────────────────────────────────────────────────────────────────────
// Huawei chain
// ──────────────────────────────────────────────────────────────────────────────

func TestCLI_HuaweiChain(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "huawei.db")
	logFile := testdataPath("huawei/teg_20260321162156.log")
	deviceID := "gz-hxy-g160304-b02-hw12816-cuf-13"

	// 1. Ingest
	out, err := runNethelper(t, dbPath, "watch", "ingest", logFile)
	if err != nil {
		t.Fatalf("watch ingest failed: %v\noutput: %s", err, out)
	}

	// 2. show device — must list the expected device
	out, err = runNethelper(t, dbPath, "show", "device")
	if err != nil {
		t.Fatalf("show device failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(strings.ToLower(out), strings.ToLower("GZ-HXY-G160304-B02-HW12816-CUF-13")) {
		t.Errorf("show device: expected device ID in output, got:\n%s", out)
	}

	// 3. show interface --device
	out, err = runNethelper(t, dbPath, "show", "interface", "--device", deviceID)
	if err != nil {
		t.Fatalf("show interface failed: %v\noutput: %s", err, out)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("show interface: expected non-empty output")
	}

	// 4. check loop — expect exit 0
	out, err = runNethelper(t, dbPath, "check", "loop")
	if err != nil {
		t.Fatalf("check loop failed: %v\noutput: %s", err, out)
	}

	// 5. export report — expect non-empty markdown
	out, err = runNethelper(t, dbPath, "export", "report")
	if err != nil {
		t.Fatalf("export report failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "# Network Status Report") {
		t.Errorf("export report: expected markdown header, got:\n%s", out)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("export report: expected non-empty output")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Cisco chain
// ──────────────────────────────────────────────────────────────────────────────

func TestCLI_CiscoChain(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cisco.db")
	logFile := testdataPath("cisco/teg_20260321162808.log")
	deviceID := "gz-ys-0101-g05-asr9912-qcstix-01"

	// 1. Ingest
	out, err := runNethelper(t, dbPath, "watch", "ingest", logFile)
	if err != nil {
		t.Fatalf("watch ingest failed: %v\noutput: %s", err, out)
	}

	// 2. show device — must list the expected device
	out, err = runNethelper(t, dbPath, "show", "device")
	if err != nil {
		t.Fatalf("show device failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(strings.ToLower(out), strings.ToLower("gz-ys-0101-g05-asr9912-qcstix-01")) {
		t.Errorf("show device: expected device ID in output, got:\n%s", out)
	}

	// 3. show interface --device
	out, err = runNethelper(t, dbPath, "show", "interface", "--device", deviceID)
	if err != nil {
		t.Fatalf("show interface failed: %v\noutput: %s", err, out)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("show interface: expected non-empty output")
	}

	// 4. check loop — expect exit 0
	out, err = runNethelper(t, dbPath, "check", "loop")
	if err != nil {
		t.Fatalf("check loop failed: %v\noutput: %s", err, out)
	}

	// 5. export report — expect non-empty markdown
	out, err = runNethelper(t, dbPath, "export", "report")
	if err != nil {
		t.Fatalf("export report failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "# Network Status Report") {
		t.Errorf("export report: expected markdown header, got:\n%s", out)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("export report: expected non-empty output")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// H3C chain
// ──────────────────────────────────────────────────────────────────────────────

func TestCLI_H3CChain(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "h3c.db")
	logFile := testdataPath("h3c/teg_20260321163710.log")
	deviceID := "gz-hxy-0203-c05-h12516xaf-qcdr-01"

	// 1. Ingest
	out, err := runNethelper(t, dbPath, "watch", "ingest", logFile)
	if err != nil {
		t.Fatalf("watch ingest failed: %v\noutput: %s", err, out)
	}

	// 2. show device — must list the expected device
	out, err = runNethelper(t, dbPath, "show", "device")
	if err != nil {
		t.Fatalf("show device failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(strings.ToLower(out), strings.ToLower("gz-hxy-0203-c05-h12516xaf-qcdr-01")) {
		t.Errorf("show device: expected device ID in output, got:\n%s", out)
	}

	// 3. show interface --device
	out, err = runNethelper(t, dbPath, "show", "interface", "--device", deviceID)
	if err != nil {
		t.Fatalf("show interface failed: %v\noutput: %s", err, out)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("show interface: expected non-empty output")
	}

	// 4. check loop — expect exit 0
	out, err = runNethelper(t, dbPath, "check", "loop")
	if err != nil {
		t.Fatalf("check loop failed: %v\noutput: %s", err, out)
	}

	// 5. export report — expect non-empty markdown
	out, err = runNethelper(t, dbPath, "export", "report")
	if err != nil {
		t.Fatalf("export report failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "# Network Status Report") {
		t.Errorf("export report: expected markdown header, got:\n%s", out)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("export report: expected non-empty output")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Juniper chain (note: log filename contains a space)
// ──────────────────────────────────────────────────────────────────────────────

func TestCLI_JuniperChain(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "juniper.db")
	// The filename has a space — filepath.Join handles it correctly; the OS
	// exec.Command passes it as a single argument so no shell escaping is needed.
	logFile := testdataPath("juniper/teg (1)_20260321162932.log")
	deviceID := "sz-bh-0701-j04-mx960-qctix-02"

	// 1. Ingest
	out, err := runNethelper(t, dbPath, "watch", "ingest", logFile)
	if err != nil {
		t.Fatalf("watch ingest failed: %v\noutput: %s", err, out)
	}

	// 2. show device — must list a device containing "sz-bh-0701"
	out, err = runNethelper(t, dbPath, "show", "device")
	if err != nil {
		t.Fatalf("show device failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(strings.ToLower(out), "sz-bh-0701") {
		t.Errorf("show device: expected device containing 'sz-bh-0701' in output, got:\n%s", out)
	}

	// 3. show interface --device
	out, err = runNethelper(t, dbPath, "show", "interface", "--device", deviceID)
	if err != nil {
		t.Fatalf("show interface failed: %v\noutput: %s", err, out)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("show interface: expected non-empty output")
	}

	// 4. check loop — expect exit 0
	out, err = runNethelper(t, dbPath, "check", "loop")
	if err != nil {
		t.Fatalf("check loop failed: %v\noutput: %s", err, out)
	}

	// 5. export report — expect non-empty markdown
	out, err = runNethelper(t, dbPath, "export", "report")
	if err != nil {
		t.Fatalf("export report failed: %v\noutput: %s", err, out)
	}
	if !strings.Contains(out, "# Network Status Report") {
		t.Errorf("export report: expected markdown header, got:\n%s", out)
	}
	if strings.TrimSpace(out) == "" {
		t.Errorf("export report: expected non-empty output")
	}
}
