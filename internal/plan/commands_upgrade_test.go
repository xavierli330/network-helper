package plan

import (
	"strings"
	"testing"
)

func TestUpgradeExecuteStep_Huawei(t *testing.T) {
	params := UpgradeParams{
		TargetVersion: "V200R021C10SPC600",
		FirmwareFile:  "NE40E-V800R021C10SPC600.cc",
	}
	dc := upgradeExecuteStep("rtr-01", params, "huawei")

	if dc.DeviceID != "rtr-01" {
		t.Errorf("DeviceID = %s, want rtr-01", dc.DeviceID)
	}

	found := map[string]bool{
		"startup system-software": false,
		"reboot":                  false,
		"save":                    false,
	}
	for _, cmd := range dc.Commands {
		for key := range found {
			if strings.Contains(cmd, key) {
				found[key] = true
			}
		}
	}
	for key, ok := range found {
		if !ok {
			t.Errorf("huawei upgrade commands missing %q", key)
		}
	}

	// Verify firmware file is referenced
	hasFirmware := false
	for _, cmd := range dc.Commands {
		if strings.Contains(cmd, params.FirmwareFile) {
			hasFirmware = true
			break
		}
	}
	if !hasFirmware {
		t.Errorf("huawei upgrade commands should reference firmware file %s", params.FirmwareFile)
	}
}

func TestUpgradeExecuteStep_H3C(t *testing.T) {
	params := UpgradeParams{
		TargetVersion: "R6728P30",
		FirmwareFile:  "s12500x-cmw710-r6728p30.ipe",
	}
	dc := upgradeExecuteStep("sw-core-01", params, "h3c")

	hasBootLoader := false
	for _, cmd := range dc.Commands {
		if strings.Contains(cmd, "boot-loader file") {
			hasBootLoader = true
			break
		}
	}
	if !hasBootLoader {
		t.Errorf("h3c upgrade commands should contain 'boot-loader file'")
	}

	hasFirmware := false
	for _, cmd := range dc.Commands {
		if strings.Contains(cmd, params.FirmwareFile) {
			hasFirmware = true
			break
		}
	}
	if !hasFirmware {
		t.Errorf("h3c upgrade commands should reference firmware file %s", params.FirmwareFile)
	}
}

func TestUpgradeExecuteStep_Cisco(t *testing.T) {
	params := UpgradeParams{
		TargetVersion: "17.09.04a",
		FirmwareFile:  "cat9k_iosxe.17.09.04a.SPA.bin",
	}
	dc := upgradeExecuteStep("sw-cisco-01", params, "cisco")

	keywords := []string{"install add", "install activate", "install commit"}
	for _, kw := range keywords {
		found := false
		for _, cmd := range dc.Commands {
			if strings.Contains(cmd, kw) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("cisco upgrade commands missing %q", kw)
		}
	}
}

func TestUpgradeExecuteStep_Juniper(t *testing.T) {
	params := UpgradeParams{
		TargetVersion: "21.4R3-S4",
		FirmwareFile:  "junos-install-mx-x86-64-21.4R3-S4.tgz",
	}
	dc := upgradeExecuteStep("mx-01", params, "juniper")

	hasInstall := false
	for _, cmd := range dc.Commands {
		if strings.Contains(cmd, "request system software add") {
			hasInstall = true
			break
		}
	}
	if !hasInstall {
		t.Errorf("juniper upgrade commands should contain 'request system software add'")
	}

	hasFirmware := false
	for _, cmd := range dc.Commands {
		if strings.Contains(cmd, params.FirmwareFile) {
			hasFirmware = true
			break
		}
	}
	if !hasFirmware {
		t.Errorf("juniper upgrade commands should reference firmware file %s", params.FirmwareFile)
	}
}

func TestUpgradeVerifyStep(t *testing.T) {
	params := UpgradeParams{
		TargetVersion: "V200R021C10SPC600",
		FirmwareFile:  "NE40E-V800R021C10SPC600.cc",
	}

	// Test huawei verify step
	dc := upgradeVerifyStep("rtr-01", params, "huawei")

	hasDisplayVersion := false
	hasVersionComment := false
	for _, cmd := range dc.Commands {
		if cmd == "display version" {
			hasDisplayVersion = true
		}
		if strings.Contains(cmd, params.TargetVersion) {
			hasVersionComment = true
		}
	}
	if !hasDisplayVersion {
		t.Errorf("huawei verify step should contain 'display version'")
	}
	if !hasVersionComment {
		t.Errorf("huawei verify step should reference target version %s", params.TargetVersion)
	}

	// Purpose should mention target version
	if !strings.Contains(dc.Purpose, params.TargetVersion) {
		t.Errorf("verify step Purpose should contain target version, got: %s", dc.Purpose)
	}
}

func TestUpgradeExecuteStep_DefaultVendor(t *testing.T) {
	params := UpgradeParams{
		TargetVersion: "2.0.0",
		FirmwareFile:  "firmware.bin",
	}
	dc := upgradeExecuteStep("dev-01", params, "unknown-vendor")

	if len(dc.Commands) == 0 {
		t.Fatal("default vendor should still produce commands")
	}
	// Should have a manual instruction comment
	hasManual := false
	for _, cmd := range dc.Commands {
		if strings.Contains(cmd, "手动执行") || strings.Contains(cmd, "#") {
			hasManual = true
			break
		}
	}
	if !hasManual {
		t.Errorf("default vendor should produce a manual instruction comment")
	}
}
