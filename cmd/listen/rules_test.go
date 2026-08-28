package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequiredRTLSDRRulesInstalledAcceptsEquivalentRulesWithExtras(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rtl-sdr.rules")
	installed := "# Distribution-managed RTL-SDR rules\n\n" + rtlRulesContent + "\n# Additional local rule\nSUBSYSTEMS==\"usb\", ATTRS{idVendor}==\"ffff\", MODE=\"0660\"\n"
	if err := os.WriteFile(path, []byte(installed), 0o644); err != nil {
		t.Fatalf("write rules file: %v", err)
	}

	installedRules, err := requiredRTLSDRRulesInstalled(path)
	if err != nil {
		t.Fatalf("requiredRTLSDRRulesInstalled() error = %v", err)
	}
	if !installedRules {
		t.Fatal("equivalent rules with extra local entries should not require installation")
	}
}

func TestRequiredRTLSDRRulesInstalledRejectsMissingRule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rtl-sdr.rules")
	missingRule := `SUBSYSTEMS=="usb", ATTRS{idVendor}=="0bda", ATTRS{idProduct}=="2832", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"`
	installed := strings.Replace(rtlRulesContent, missingRule, "", 1)
	if err := os.WriteFile(path, []byte(installed), 0o644); err != nil {
		t.Fatalf("write rules file: %v", err)
	}

	installedRules, err := requiredRTLSDRRulesInstalled(path)
	if err != nil {
		t.Fatalf("requiredRTLSDRRulesInstalled() error = %v", err)
	}
	if installedRules {
		t.Fatal("missing required RTL-SDR rule should require installation")
	}
}
