package model

import "testing"

func TestInterfaceTypeValid(t *testing.T) {
	valid := []InterfaceType{
		IfTypePhysical, IfTypeLoopback, IfTypeVlanif,
		IfTypeEthTrunk, IfTypeTunnelTE, IfTypeTunnelSR,
		IfTypeTunnelGRE, IfTypeNVE, IfTypeNull, IfTypeSubInterface,
	}
	for _, v := range valid {
		if !v.Valid() {
			t.Errorf("expected %q to be valid", v)
		}
	}
	if InterfaceType("bogus").Valid() {
		t.Error("expected 'bogus' to be invalid")
	}
}

func TestDeviceID(t *testing.T) {
	d := Device{Hostname: "Core-01", Vendor: "huawei"}
	if d.Hostname == "" {
		t.Error("hostname should not be empty")
	}
}
