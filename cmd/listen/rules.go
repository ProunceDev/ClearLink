package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	rtlRulesDestPath = "/etc/udev/rules.d/rtl-sdr.rules"
	localRulesName   = "rtl-sdr.rules"
)

const rtlRulesContent = `#
# Copyright 2012-2013 Osmocom rtl-sdr project
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.
#

# original RTL2832U vid/pid (hama nano, for example)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0bda", ATTRS{idProduct}=="2832", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# RTL2832U OEM vid/pid, e.g. ezcap EzTV668 (E4000), Newsky TV28T (E4000/R820T) etc.
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0bda", ATTRS{idProduct}=="2838", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# DigitalNow Quad DVB-T PCI-E card (4x FC0012?)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0413", ATTRS{idProduct}=="6680", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Leadtek WinFast DTV Dongle mini D (FC0012)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0413", ATTRS{idProduct}=="6f0f", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Genius TVGo DVB-T03 USB dongle (Ver. B)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0458", ATTRS{idProduct}=="707f", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Terratec Cinergy T Stick Black (rev 1) (FC0012)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0ccd", ATTRS{idProduct}=="00a9", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Terratec NOXON rev 1 (FC0013)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0ccd", ATTRS{idProduct}=="00b3", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Terratec Deutschlandradio DAB Stick (FC0013)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0ccd", ATTRS{idProduct}=="00b4", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Terratec NOXON DAB Stick - Radio Energy (FC0013)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0ccd", ATTRS{idProduct}=="00b5", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Terratec Media Broadcast DAB Stick (FC0013)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0ccd", ATTRS{idProduct}=="00b7", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Terratec BR DAB Stick (FC0013)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0ccd", ATTRS{idProduct}=="00b8", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Terratec WDR DAB Stick (FC0013)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0ccd", ATTRS{idProduct}=="00b9", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Terratec MuellerVerlag DAB Stick (FC0013)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0ccd", ATTRS{idProduct}=="00c0", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Terratec Fraunhofer DAB Stick (FC0013)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0ccd", ATTRS{idProduct}=="00c6", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Terratec Cinergy T Stick RC (Rev.3) (E4000)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0ccd", ATTRS{idProduct}=="00d3", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Terratec T Stick PLUS (E4000)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0ccd", ATTRS{idProduct}=="00d7", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Terratec NOXON rev 2 (E4000)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="0ccd", ATTRS{idProduct}=="00e0", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# PixelView PV-DT235U(RN) (FC0012)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1554", ATTRS{idProduct}=="5020", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Astrometa DVB-T/DVB-T2 (R828D)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="15f4", ATTRS{idProduct}=="0131", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# HanfTek DAB+FM+DVB-T
SUBSYSTEMS=="usb", ATTRS{idVendor}=="15f4", ATTRS{idProduct}=="0133", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Compro Videomate U620F (E4000)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="185b", ATTRS{idProduct}=="0620", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Compro Videomate U650F (E4000)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="185b", ATTRS{idProduct}=="0650", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Compro Videomate U680F (E4000)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="185b", ATTRS{idProduct}=="0680", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# GIGABYTE GT-U7300 (FC0012)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1b80", ATTRS{idProduct}=="d393", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# DIKOM USB-DVBT HD
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1b80", ATTRS{idProduct}=="d394", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Peak 102569AGPK (FC0012)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1b80", ATTRS{idProduct}=="d395", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# KWorld KW-UB450-T USB DVB-T Pico TV (TUA9001)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1b80", ATTRS{idProduct}=="d397", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Zaapa ZT-MINDVBZP (FC0012)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1b80", ATTRS{idProduct}=="d398", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# SVEON STV20 DVB-T USB & FM (FC0012)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1b80", ATTRS{idProduct}=="d39d", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Twintech UT-40 (FC0013)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1b80", ATTRS{idProduct}=="d3a4", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# ASUS U3100MINI_PLUS_V2 (FC0013)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1b80", ATTRS{idProduct}=="d3a8", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# SVEON STV27 DVB-T USB & FM (FC0013)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1b80", ATTRS{idProduct}=="d3af", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# SVEON STV21 DVB-T USB & FM
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1b80", ATTRS{idProduct}=="d3b0", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Dexatek DK DVB-T Dongle (Logilink VG0002A) (FC2580)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1d19", ATTRS{idProduct}=="1101", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Dexatek DK DVB-T Dongle (MSI DigiVox mini II V3.0)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1d19", ATTRS{idProduct}=="1102", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Dexatek DK 5217 DVB-T Dongle (FC2580)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1d19", ATTRS{idProduct}=="1103", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# MSI DigiVox Micro HD (FC2580)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1d19", ATTRS{idProduct}=="1104", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Sweex DVB-T USB (FC0012)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1f4d", ATTRS{idProduct}=="a803", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# GTek T803 (FC0012)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1f4d", ATTRS{idProduct}=="b803", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# Lifeview LV5TDeluxe (FC0012)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1f4d", ATTRS{idProduct}=="c803", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# MyGica TD312 (FC0012)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1f4d", ATTRS{idProduct}=="d286", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"

# PROlectrix DV107669 (FC0012)
SUBSYSTEMS=="usb", ATTRS{idVendor}=="1f4d", ATTRS{idProduct}=="d803", ENV{ID_SOFTWARE_RADIO}="1", MODE="0660", GROUP="plugdev"
`

func installRTLSDRRules() error {
	if os.Getenv("CLEARLINK_SKIP_RTL_RULES_INSTALL") == "1" {
		return nil
	}

	alreadyInstalled, err := sameContentAsInstalled(rtlRulesDestPath)
	if err == nil && alreadyInstalled {
		return nil
	}

	tmpFile, err := os.CreateTemp("", "rtl-sdr-*.rules")
	if err != nil {
		return fmt.Errorf("failed to create temporary RTL-SDR rules file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(rtlRulesContent); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temporary RTL-SDR rules file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary RTL-SDR rules file: %w", err)
	}

	if err := runPrivilegedCommand("cp", tmpPath, rtlRulesDestPath); err != nil {
		return installFailureWithManualCommands(fmt.Errorf("failed to copy RTL-SDR rules to %s: %w", rtlRulesDestPath, err))
	}

	if err := runPrivilegedCommand("udevadm", "control", "--reload-rules"); err != nil {
		return installFailureWithManualCommands(fmt.Errorf("failed to reload udev rules: %w", err))
	}

	if err := runPrivilegedCommand("udevadm", "trigger"); err != nil {
		return installFailureWithManualCommands(fmt.Errorf("failed to trigger udev after rules reload: %w", err))
	}

	return nil
}

func runPrivilegedCommand(command string, args ...string) error {
	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.Command(command, args...)
	} else {
		sudoArgs := append([]string{command}, args...)
		cmd = exec.Command("sudo", sudoArgs...)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func installFailureWithManualCommands(cause error) error {
	rulesPath, writeErr := writeLocalRulesFile()
	if writeErr != nil {
		return fmt.Errorf("%w\nAlso failed to save local rules file: %v\nManual commands:\n1) create a file named %s with RTL-SDR rules\n2) sudo cp <path-to-%s> %s\n3) sudo udevadm control --reload-rules\n4) sudo udevadm trigger", cause, writeErr, localRulesName, localRulesName, rtlRulesDestPath)
	}

	commands := []string{
		"sudo cp " + rulesPath + " " + rtlRulesDestPath,
		"sudo udevadm control --reload-rules",
		"sudo udevadm trigger",
	}

	return fmt.Errorf("%w\nSaved rules file: %s\nManual recovery commands:\n%s", cause, rulesPath, numberedCommands(commands))
}

func writeLocalRulesFile() (string, error) {
	path := localRulesName
	if wd, err := os.Getwd(); err == nil {
		path = filepath.Join(wd, localRulesName)
	}
	if err := os.WriteFile(path, []byte(rtlRulesContent), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func numberedCommands(commands []string) string {
	var b strings.Builder
	for i, cmd := range commands {
		b.WriteString(fmt.Sprintf("%d) %s", i+1, cmd))
		if i < len(commands)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func sameContentAsInstalled(destPath string) (bool, error) {
	dest, err := os.ReadFile(destPath)
	if err != nil {
		return false, err
	}
	return bytes.Equal([]byte(rtlRulesContent), dest), nil
}
