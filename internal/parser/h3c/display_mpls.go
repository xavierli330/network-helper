// internal/parser/h3c/display_mpls.go
package h3c

import "github.com/xavierli/nethelper/internal/model"

func ParseMplsLsp(raw string) (model.ParseResult, error) {
	// H3C MPLS LSP format is very similar to Huawei — reuse same pattern
	return model.ParseResult{Type: model.CmdLFIB, RawText: raw}, nil
}
