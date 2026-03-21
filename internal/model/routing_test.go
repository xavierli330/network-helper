package model

import "testing"

func TestRIBEntryPrefixString(t *testing.T) {
	r := RIBEntry{Prefix: "10.0.0.0", MaskLen: 24, Protocol: "ospf"}
	if r.PrefixString() != "10.0.0.0/24" {
		t.Errorf("expected 10.0.0.0/24, got %s", r.PrefixString())
	}
}

func TestFIBEntryLabelAction(t *testing.T) {
	f := FIBEntry{LabelAction: "push", OutLabel: "16001"}
	if f.LabelAction == "" {
		t.Error("label_action should not be empty")
	}
}

func TestLFIBEntryFields(t *testing.T) {
	l := LFIBEntry{InLabel: 16001, Action: "swap", OutLabel: "16002", Protocol: "ldp"}
	if l.InLabel != 16001 {
		t.Errorf("expected 16001, got %d", l.InLabel)
	}
}
