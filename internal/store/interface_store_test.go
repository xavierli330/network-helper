package store

import (
	"testing"
	"time"
	"github.com/xavierli/nethelper/internal/model"
)

func TestUpsertAndGetInterfaces(t *testing.T) {
	db := testDB(t)
	db.UpsertDevice(model.Device{ID: "d1", Hostname: "D1", Vendor: "huawei", LastSeen: time.Now()})
	iface := model.Interface{
		ID: "d1:GE0/0/1", DeviceID: "d1", Name: "GE0/0/1",
		Type: model.IfTypePhysical, Status: "up",
		IPAddress: "10.0.0.1", Mask: "255.255.255.0", LastUpdated: time.Now(),
	}
	if err := db.UpsertInterface(iface); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	ifaces, err := db.GetInterfaces("d1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(ifaces) != 1 || ifaces[0].Name != "GE0/0/1" {
		t.Errorf("unexpected: %+v", ifaces)
	}
}
