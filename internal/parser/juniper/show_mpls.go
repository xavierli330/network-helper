// internal/parser/juniper/show_mpls.go
package juniper

import "github.com/xavierli/nethelper/internal/model"

func ParseShowMplsRoute(raw string) (model.ParseResult, error) {
	return model.ParseResult{Type: model.CmdLFIB, RawText: raw}, nil
}
