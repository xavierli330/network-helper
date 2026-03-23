package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SessionEvent is a single event in the session log.
type SessionEvent struct {
	Timestamp  string                 `json:"ts"`
	UserKey    string                 `json:"user,omitempty"`
	Type       string                 `json:"type"` // "user", "assistant", "tool_call", "tool_result", "memory", "error"
	Content    string                 `json:"content,omitempty"`
	ToolName   string                 `json:"tool,omitempty"`
	ToolArgs   map[string]interface{} `json:"args,omitempty"`
	DurationMs int64                  `json:"duration_ms,omitempty"`
}

// SessionLogger writes JSONL audit logs, one file per user per day.
type SessionLogger struct {
	dir string
	mu  sync.Mutex
}

// NewSessionLogger creates a SessionLogger that writes under <dataDir>/sessions/.
func NewSessionLogger(dataDir string) *SessionLogger {
	dir := filepath.Join(dataDir, "sessions")
	os.MkdirAll(dir, 0755)
	return &SessionLogger{dir: dir}
}

// Log appends an event to the user's daily JSONL file.
// It is safe for concurrent use and is a no-op when l is nil.
func (l *SessionLogger) Log(userKey string, event SessionEvent) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	event.Timestamp = time.Now().Format(time.RFC3339)
	if event.UserKey == "" {
		event.UserKey = userKey
	}

	// Filename: <sanitized_user_key>_<date>.jsonl
	date := time.Now().Format("2006-01-02")
	safeName := sanitizeFilename(userKey)
	path := filepath.Join(l.dir, fmt.Sprintf("%s_%s.jsonl", safeName, date))

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	data, _ := json.Marshal(event)
	f.Write(data)
	f.Write([]byte("\n"))
}

// sanitizeFilename replaces any character that is not alphanumeric, '-', or '_'
// with an underscore so the result is safe to use as a filename component.
func sanitizeFilename(s string) string {
	result := make([]byte, 0, len(s))
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, byte(c))
		} else {
			result = append(result, '_')
		}
	}
	if len(result) == 0 {
		return "unknown"
	}
	return string(result)
}
