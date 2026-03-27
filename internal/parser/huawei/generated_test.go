// internal/parser/huawei/generated_test.go
package huawei_test

import (
	"testing"
	"github.com/xavierli/nethelper/internal/model"
	"github.com/xavierli/nethelper/internal/parser/huawei"
)

// TestClassifyCommandUnknownBaseline is a non-regression anchor: unknown commands return
// CmdUnknown both before and after the fallback hook is wired.
func TestClassifyCommandUnknownBaseline(t *testing.T) {
	p := huawei.New()
	ct := p.ClassifyCommand("display traffic-policy")
	if ct != model.CmdUnknown {
		t.Errorf("expected CmdUnknown, got %v", ct)
	}
}
