package sdrhelper

import (
	"os"
	"strings"
)

func RaspberryPiGeneration() string {
	data, err := os.ReadFile("/proc/device-tree/model")
	if err != nil {
		return "unknown"
	}

	model := strings.TrimRight(string(data), "\x00\n")

	switch {
	case strings.Contains(model, "Raspberry Pi 3"):
		return "pi3"

	case strings.Contains(model, "Raspberry Pi 4"),
		strings.Contains(model, "Raspberry Pi 400"),
		strings.Contains(model, "Raspberry Pi 5"),
		strings.Contains(model, "Raspberry Pi 500"):
		return "pi4+"

	default:
		return "other"
	}
}