package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"gopkg.in/ini.v1"
)

func (m *Manager) ensureRuntimeDependencies() error {
	if runtime.GOOS != "linux" {
		return nil
	}

	commands, err := m.requiredALSACommands()
	if err != nil {
		return err
	}
	missing := missingCommands(commands, exec.LookPath)
	if len(missing) == 0 {
		return nil
	}

	info("Installing alsa-utils; missing command(s): %s", strings.Join(missing, ", "))
	if err := installALSAUtils(); err != nil {
		return err
	}

	if missing = missingCommands(commands, exec.LookPath); len(missing) != 0 {
		return fmt.Errorf("alsa-utils installed but command(s) are still unavailable: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (m *Manager) requiredALSACommands() ([]string, error) {
	switch m.config.LinkType {
	case "interface":
		return []string{"arecord", "aplay"}, nil
	case "broadcast":
		linkConfig, err := ini.Load(m.linkConfigPath)
		if err != nil {
			return nil, fmt.Errorf("read broadcast config: %w", err)
		}
		broadcastType := strings.ToUpper(strings.TrimSpace(linkConfig.Section("broadcast").Key("Type").MustString("DISCORD")))
		if broadcastType == "RADIO" {
			return []string{"aplay"}, nil
		}
	}
	return nil, nil
}

func missingCommands(commands []string, lookup func(string) (string, error)) []string {
	missing := make([]string, 0, len(commands))
	for _, command := range commands {
		if _, err := lookup(command); err != nil {
			missing = append(missing, command)
		}
	}
	return missing
}

func installALSAUtils() error {
	if _, err := exec.LookPath("apt-get"); err != nil {
		return fmt.Errorf("alsa-utils is required but apt-get is unavailable: %w", err)
	}

	if err := runPrivilegedSystemCommand("apt-get", "update"); err != nil {
		return fmt.Errorf("update package index before installing alsa-utils: %w", err)
	}
	if err := runPrivilegedSystemCommand("apt-get", "install", "-y", "alsa-utils"); err != nil {
		return fmt.Errorf("install alsa-utils: %w", err)
	}
	return nil
}

func runPrivilegedSystemCommand(command string, args ...string) error {
	commandArgs := args
	commandName := command
	if os.Geteuid() != 0 {
		if _, err := exec.LookPath("sudo"); err != nil {
			return fmt.Errorf("sudo is required to install system packages: %w", err)
		}
		commandName = "sudo"
		commandArgs = append([]string{command}, args...)
	}

	cmd := exec.Command(commandName, commandArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
