package parser

import (
	"testing"
	"time"
)

func TestSplitBlocks_Huawei(t *testing.T) {
	input := "<HUAWEI-Core-01>display ip routing-table\nRoute Flags: R - relay\n10.1.1.0/24         OSPF    10   2           10.0.0.1        GE0/0/1\n<HUAWEI-Core-01>display interface brief\nInterface                   PHY   Protocol\nGE0/0/1                     up    up\n"

	registry := NewRegistry()
	registry.Register(newPromptOnlyParser("huawei", `^<([^>]+)>`))

	blocks := Split(input, registry)
	if len(blocks) != 2 { t.Fatalf("expected 2 blocks, got %d", len(blocks)) }
	if blocks[0].Hostname != "HUAWEI-Core-01" { t.Errorf("block 0 hostname: got %s", blocks[0].Hostname) }
	if blocks[0].Command != "display ip routing-table" { t.Errorf("block 0 cmd: got %q", blocks[0].Command) }
	if blocks[1].Command != "display interface brief" { t.Errorf("block 1 cmd: got %q", blocks[1].Command) }
}

func TestSplitBlocks_Cisco(t *testing.T) {
	input := "Router-PE01#show ip route\nCodes: L - local\n     10.0.0.0/8\nRouter-PE01#show interfaces brief\nInterface              IP-Address\n"

	registry := NewRegistry()
	registry.Register(newPromptOnlyParser("cisco", `^([A-Za-z][A-Za-z0-9._-]*)#`))

	blocks := Split(input, registry)
	if len(blocks) != 2 { t.Fatalf("expected 2 blocks, got %d", len(blocks)) }
	if blocks[0].Hostname != "Router-PE01" { t.Errorf("expected Router-PE01, got %s", blocks[0].Hostname) }
}

func TestSplitBlocks_Empty(t *testing.T) {
	registry := NewRegistry()
	blocks := Split("", registry)
	if len(blocks) != 0 { t.Errorf("expected 0, got %d", len(blocks)) }
}

func TestSplitBlocks_Juniper(t *testing.T) {
	input := "admin@MX204-01> show route\ninet.0: 5 destinations\nadmin@MX204-01> show interfaces terse\nInterface               Admin Link\n"

	registry := NewRegistry()
	registry.Register(newPromptOnlyParser("juniper", `^[a-zA-Z][a-zA-Z0-9._-]*@([A-Za-z][A-Za-z0-9._-]*)>`))

	blocks := Split(input, registry)
	if len(blocks) != 2 { t.Fatalf("expected 2, got %d", len(blocks)) }
	if blocks[0].Hostname != "MX204-01" { t.Errorf("expected MX204-01, got %s", blocks[0].Hostname) }
}

func TestExtractTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantOK  bool
		wantVal string // "YYYY-MM-DD HH:MM:SS" format for comparison, empty if wantOK=false
	}{
		{
			name:    "dash-separated format",
			line:    "2026-03-21-16-22-26: <Router>display version",
			wantOK:  true,
			wantVal: "2026-03-21 16:22:26",
		},
		{
			name:    "space-colon format",
			line:    "2026-03-21 16:22:26: <Router>display version",
			wantOK:  true,
			wantVal: "2026-03-21 16:22:26",
		},
		{
			name:   "no timestamp",
			line:   "<Router>display version",
			wantOK: false,
		},
		{
			name:   "empty line",
			line:   "",
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := extractTimestamp(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("extractTimestamp(%q): ok=%v, want %v", tc.line, ok, tc.wantOK)
			}
			if tc.wantOK {
				want, err := time.ParseInLocation("2006-01-02 15:04:05", tc.wantVal, time.Local)
				if err != nil {
					t.Fatalf("bad wantVal in test: %v", err)
				}
				if !got.Equal(want) {
					t.Errorf("extractTimestamp(%q): got %v, want %v", tc.line, got, want)
				}
			}
		})
	}
}

func TestSplitBlocks_TimestampExtracted(t *testing.T) {
	// Log lines with "2026-03-21-16-22-26: " prefix before the prompt.
	input := "2026-03-21-16-22-26: <Core-01>display version\nHuawei VRP V200R020\n" +
		"2026-03-21-16-22-30: <Core-01>display interface brief\nInterface  PHY\n"

	registry := NewRegistry()
	registry.Register(newPromptOnlyParser("huawei", `^<([^>]+)>`))

	blocks := Split(input, registry)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}

	// Verify timestamp was extracted on the first block.
	want, _ := time.ParseInLocation("2006-01-02-15-04-05", "2026-03-21-16-22-26", time.Local)
	if blocks[0].CapturedAt.IsZero() {
		t.Error("block 0 CapturedAt should not be zero")
	} else if !blocks[0].CapturedAt.Equal(want) {
		t.Errorf("block 0 CapturedAt: got %v, want %v", blocks[0].CapturedAt, want)
	}

	// Verify the prompt was stripped from the command (no timestamp bleed).
	if blocks[0].Command != "display version" {
		t.Errorf("block 0 command: got %q", blocks[0].Command)
	}
	// Verify the hostname is clean.
	if blocks[0].Hostname != "Core-01" {
		t.Errorf("block 0 hostname: got %q", blocks[0].Hostname)
	}
}

func TestSplitBlocks_NoTimestamp_CapturedAtZero(t *testing.T) {
	input := "<Core-01>display version\nHuawei VRP V200R020\n"

	registry := NewRegistry()
	registry.Register(newPromptOnlyParser("huawei", `^<([^>]+)>`))

	blocks := Split(input, registry)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if !blocks[0].CapturedAt.IsZero() {
		t.Errorf("CapturedAt should be zero when no timestamp in log, got %v", blocks[0].CapturedAt)
	}
}

