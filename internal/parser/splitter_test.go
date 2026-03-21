package parser

import "testing"

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
