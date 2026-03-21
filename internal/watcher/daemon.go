// internal/watcher/daemon.go
package watcher

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// WritePID writes the current process PID to a file.
func WritePID(path string) error {
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0644)
}

// ReadPID reads a PID from a file.
func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil { return 0, err }
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil { return 0, fmt.Errorf("invalid PID in %s: %w", path, err) }
	return pid, nil
}

// RemovePID removes the PID file.
func RemovePID(path string) {
	os.Remove(path)
}

// IsRunning checks if a process with the PID in the file is still running.
func IsRunning(path string) bool {
	pid, err := ReadPID(path)
	if err != nil { return false }

	process, err := os.FindProcess(pid)
	if err != nil { return false }

	// On Unix, FindProcess always succeeds. Send signal 0 to check if alive.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
