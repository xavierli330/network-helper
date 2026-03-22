package parser

import (
	"testing"

	"github.com/xavierli/nethelper/internal/model"
)

func TestExtractInterfaces_HuaweiConfig(t *testing.T) {
	config := `#
sysname Core-SW01
#
interface GigabitEthernet1/0/0
 description To-Core-Switch
 ip address 10.1.1.1 255.255.255.252
 undo shutdown
#
interface LoopBack0
 ip address 1.1.1.1 255.255.255.255
#
interface Eth-Trunk1
 description LAG-to-Spine
 ip address 172.16.0.1 255.255.255.254
#
interface Vlanif100
 ip address 192.168.1.1 255.255.255.0
 shutdown
#
interface GigabitEthernet1/0/1
#
`
	ifaces := ExtractInterfacesFromConfig(config, "huawei")

	if len(ifaces) != 5 {
		t.Fatalf("expected 5 interfaces, got %d", len(ifaces))
	}

	tests := []struct {
		name   string
		ifType model.InterfaceType
		ip     string
		mask   string
		desc   string
		status string
	}{
		{"GigabitEthernet1/0/0", model.IfTypePhysical, "10.1.1.1", "255.255.255.252", "To-Core-Switch", "up"},
		{"LoopBack0", model.IfTypeLoopback, "1.1.1.1", "255.255.255.255", "", "up"},
		{"Eth-Trunk1", model.IfTypeEthTrunk, "172.16.0.1", "255.255.255.254", "LAG-to-Spine", "up"},
		{"Vlanif100", model.IfTypeVlanif, "192.168.1.1", "255.255.255.0", "", "down"},
		{"GigabitEthernet1/0/1", model.IfTypePhysical, "", "", "", "up"},
	}

	for i, tc := range tests {
		iface := ifaces[i]
		if iface.Name != tc.name {
			t.Errorf("[%d] name: got %q, want %q", i, iface.Name, tc.name)
		}
		if iface.Type != tc.ifType {
			t.Errorf("[%d] %s type: got %q, want %q", i, tc.name, iface.Type, tc.ifType)
		}
		if iface.IPAddress != tc.ip {
			t.Errorf("[%d] %s ip: got %q, want %q", i, tc.name, iface.IPAddress, tc.ip)
		}
		if iface.Mask != tc.mask {
			t.Errorf("[%d] %s mask: got %q, want %q", i, tc.name, iface.Mask, tc.mask)
		}
		if iface.Description != tc.desc {
			t.Errorf("[%d] %s desc: got %q, want %q", i, tc.name, iface.Description, tc.desc)
		}
		if iface.Status != tc.status {
			t.Errorf("[%d] %s status: got %q, want %q", i, tc.name, iface.Status, tc.status)
		}
	}
}

func TestExtractInterfaces_H3CConfig(t *testing.T) {
	config := `#
sysname H3C-Core
#
interface GigabitEthernet1/0/1
 port link-mode route
 description To-Core
 ip address 10.2.1.1 255.255.255.252
#
interface LoopBack0
 ip address 2.2.2.2 255.255.255.255
#
interface Bridge-Aggregation1
 description LAG-to-Spine
 ip address 172.16.0.1 255.255.255.254
#
`
	ifaces := ExtractInterfacesFromConfig(config, "h3c")

	if len(ifaces) != 3 {
		t.Fatalf("expected 3 interfaces, got %d", len(ifaces))
	}

	if ifaces[0].Name != "GigabitEthernet1/0/1" {
		t.Errorf("name: got %q", ifaces[0].Name)
	}
	if ifaces[0].Type != model.IfTypePhysical {
		t.Errorf("type: got %q", ifaces[0].Type)
	}
	if ifaces[0].IPAddress != "10.2.1.1" {
		t.Errorf("ip: got %q", ifaces[0].IPAddress)
	}
	if ifaces[0].Description != "To-Core" {
		t.Errorf("desc: got %q", ifaces[0].Description)
	}

	if ifaces[1].Type != model.IfTypeLoopback {
		t.Errorf("LoopBack0 type: got %q", ifaces[1].Type)
	}

	if ifaces[2].Type != model.IfTypeEthTrunk {
		t.Errorf("Bridge-Aggregation1 type: got %q, want eth-trunk", ifaces[2].Type)
	}
}

func TestExtractInterfaces_CiscoConfig(t *testing.T) {
	config := `interface GigabitEthernet0/0/0/0
 description To-Core
 ipv4 address 10.3.1.1 255.255.255.252
!
interface Loopback0
 ipv4 address 3.3.3.3 255.255.255.255
!
interface Bundle-Ether1
 description LAG-to-Spine
 ipv4 address 172.16.0.1 255.255.255.254
 shutdown
!
`
	ifaces := ExtractInterfacesFromConfig(config, "cisco")

	if len(ifaces) != 3 {
		t.Fatalf("expected 3 interfaces, got %d", len(ifaces))
	}

	tests := []struct {
		name   string
		ifType model.InterfaceType
		ip     string
		desc   string
		status string
	}{
		{"GigabitEthernet0/0/0/0", model.IfTypePhysical, "10.3.1.1", "To-Core", "up"},
		{"Loopback0", model.IfTypeLoopback, "3.3.3.3", "", "up"},
		{"Bundle-Ether1", model.IfTypeEthTrunk, "172.16.0.1", "LAG-to-Spine", "down"},
	}

	for i, tc := range tests {
		iface := ifaces[i]
		if iface.Name != tc.name {
			t.Errorf("[%d] name: got %q, want %q", i, iface.Name, tc.name)
		}
		if iface.Type != tc.ifType {
			t.Errorf("[%d] %s type: got %q, want %q", i, tc.name, iface.Type, tc.ifType)
		}
		if iface.IPAddress != tc.ip {
			t.Errorf("[%d] %s ip: got %q, want %q", i, tc.name, iface.IPAddress, tc.ip)
		}
		if iface.Description != tc.desc {
			t.Errorf("[%d] %s desc: got %q, want %q", i, tc.name, iface.Description, tc.desc)
		}
		if iface.Status != tc.status {
			t.Errorf("[%d] %s status: got %q, want %q", i, tc.name, iface.Status, tc.status)
		}
	}
}

func TestExtractInterfaces_JuniperConfig(t *testing.T) {
	config := `interfaces {
    ge-0/0/0 {
        description "To-Core";
        unit 0 {
            family inet {
                address 10.4.1.1/30;
            }
        }
    }
    lo0 {
        unit 0 {
            family inet {
                address 4.4.4.4/32;
            }
        }
    }
    ae0 {
        description "LAG-to-Spine";
        unit 0 {
            family inet {
                address 172.16.0.1/31;
            }
        }
    }
}
`
	ifaces := ExtractInterfacesFromConfig(config, "juniper")

	if len(ifaces) != 3 {
		t.Fatalf("expected 3 interfaces, got %d", len(ifaces))
	}

	tests := []struct {
		name   string
		ifType model.InterfaceType
		ip     string
		mask   string
		desc   string
	}{
		{"ge-0/0/0.0", model.IfTypePhysical, "10.4.1.1", "30", "To-Core"},
		{"lo0.0", model.IfTypeLoopback, "4.4.4.4", "32", ""},
		{"ae0.0", model.IfTypeEthTrunk, "172.16.0.1", "31", "LAG-to-Spine"},
	}

	for i, tc := range tests {
		iface := ifaces[i]
		if iface.Name != tc.name {
			t.Errorf("[%d] name: got %q, want %q", i, iface.Name, tc.name)
		}
		if iface.Type != tc.ifType {
			t.Errorf("[%d] %s type: got %q, want %q", i, tc.name, iface.Type, tc.ifType)
		}
		if iface.IPAddress != tc.ip {
			t.Errorf("[%d] %s ip: got %q, want %q", i, tc.name, iface.IPAddress, tc.ip)
		}
		if iface.Mask != tc.mask {
			t.Errorf("[%d] %s mask: got %q, want %q", i, tc.name, iface.Mask, tc.mask)
		}
		if iface.Description != tc.desc {
			t.Errorf("[%d] %s desc: got %q, want %q", i, tc.name, iface.Description, tc.desc)
		}
	}
}

func TestExtractInterfaces_EmptyConfig(t *testing.T) {
	ifaces := ExtractInterfacesFromConfig("", "huawei")
	if len(ifaces) != 0 {
		t.Errorf("expected 0 interfaces from empty config, got %d", len(ifaces))
	}
}

func TestExtractInterfaces_UnknownVendor(t *testing.T) {
	ifaces := ExtractInterfacesFromConfig("interface Foo\n ip address 1.2.3.4 255.0.0.0", "unknown")
	if ifaces != nil {
		t.Errorf("expected nil for unknown vendor, got %d interfaces", len(ifaces))
	}
}

func TestExtractInterfaces_JuniperNoInterfacesBlock(t *testing.T) {
	config := `system {
    host-name router1;
}
`
	ifaces := ExtractInterfacesFromConfig(config, "juniper")
	if len(ifaces) != 0 {
		t.Errorf("expected 0 interfaces, got %d", len(ifaces))
	}
}

func TestInferInterfaceTypeByVendor(t *testing.T) {
	tests := []struct {
		name   string
		vendor string
		want   model.InterfaceType
	}{
		{"GigabitEthernet1/0/0", "huawei", model.IfTypePhysical},
		{"LoopBack0", "huawei", model.IfTypeLoopback},
		{"Vlanif100", "huawei", model.IfTypeVlanif},
		{"Eth-Trunk1", "huawei", model.IfTypeEthTrunk},
		{"Bridge-Aggregation1", "h3c", model.IfTypeEthTrunk},
		{"Loopback0", "cisco", model.IfTypeLoopback},
		{"Bundle-Ether1", "cisco", model.IfTypeEthTrunk},
		{"lo0", "juniper", model.IfTypeLoopback},
		{"ae0", "juniper", model.IfTypeEthTrunk},
		{"ge-0/0/0", "juniper", model.IfTypePhysical},
	}

	for _, tc := range tests {
		got := inferInterfaceTypeByVendor(tc.name, tc.vendor)
		if got != tc.want {
			t.Errorf("inferInterfaceTypeByVendor(%q, %q) = %q, want %q", tc.name, tc.vendor, got, tc.want)
		}
	}
}
