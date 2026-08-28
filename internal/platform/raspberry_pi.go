package platform

import (
	"os"
	"strings"
)

const raspberryPiModelPath = "/proc/device-tree/model"

// IsRaspberryPi reports whether the current host identifies as a Raspberry Pi.
func IsRaspberryPi() bool {
	model, err := os.ReadFile(raspberryPiModelPath)
	if err != nil {
		return false
	}
	return IsRaspberryPiModel(string(model))
}

func IsRaspberryPiModel(model string) bool {
	model = strings.TrimRight(model, "\x00\n")
	return strings.Contains(strings.ToLower(model), "raspberry pi")
}
