// internal/discovery/engine_test.go
package discovery_test

import (
	"fmt"
	"testing"

	"github.com/xavierli/nethelper/internal/discovery"
	"github.com/xavierli/nethelper/internal/store"
)

func TestClusterGroups(t *testing.T) {
	db, _ := store.Open(":memory:")
	defer db.Close()

	for i := 0; i < 3; i++ {
		db.UpsertUnknownOutput(store.UnknownOutput{
			DeviceID: "dev1", Vendor: "huawei",
			CommandRaw: "display traffic-policy", CommandNorm: "display traffic-policy",
			RawOutput:   fmt.Sprintf("output variant %d", i),
			ContentHash: fmt.Sprintf("hash%d", i),
		})
	}
	db.UpsertUnknownOutput(store.UnknownOutput{
		DeviceID: "dev2", Vendor: "huawei",
		CommandRaw: "display qos", CommandNorm: "display qos",
		RawOutput: "qos output", ContentHash: "hashqos",
	})

	groups := discovery.ClusterByCommand(db, "huawei")
	if len(groups) != 2 {
		t.Fatalf("want 2 command groups, got %d", len(groups))
	}
}
