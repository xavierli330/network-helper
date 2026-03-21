package model

import "testing"

func TestCommandTypeString(t *testing.T) {
	tests := []struct {
		ct   CommandType
		want string
	}{
		{CmdRIB, "rib"},
		{CmdFIB, "fib"},
		{CmdLFIB, "lfib"},
		{CmdInterface, "interface"},
		{CmdNeighbor, "neighbor"},
		{CmdTunnel, "tunnel"},
		{CmdSRMapping, "sr_mapping"},
		{CmdConfig, "config"},
		{CmdUnknown, "unknown"},
	}
	for _, tt := range tests {
		if string(tt.ct) != tt.want {
			t.Errorf("expected %q, got %q", tt.want, string(tt.ct))
		}
	}
}

func TestParseResultIsEmpty(t *testing.T) {
	pr := ParseResult{}
	if !pr.IsEmpty() {
		t.Error("empty ParseResult should report IsEmpty=true")
	}
	pr.RIBEntries = []RIBEntry{{Prefix: "10.0.0.0"}}
	if pr.IsEmpty() {
		t.Error("non-empty ParseResult should report IsEmpty=false")
	}
}

func TestSnapshotFields(t *testing.T) {
	s := Snapshot{DeviceID: "dev1", SourceFile: "/tmp/log.txt"}
	if s.DeviceID != "dev1" {
		t.Error("unexpected device_id")
	}
}
