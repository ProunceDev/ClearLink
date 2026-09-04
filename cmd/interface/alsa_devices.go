package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const alsaProcPath = "/proc/asound"

type alsaPCMDevice struct {
	cardNumber   int
	deviceNumber int
	cardID       string
}

func displayALSAPCMDevices() {
	inputDevices, outputDevices, err := discoverALSAPCMDeviceNames(alsaProcPath, os.ReadFile)
	if err != nil {
		log.Printf("Unable to enumerate ALSA devices: %v", err)
		return
	}
	printALSAPCMDevices(os.Stdout, inputDevices, outputDevices)
}

func discoverALSAPCMDeviceNames(procPath string, readFile func(string) ([]byte, error)) ([]string, []string, error) {
	cardIDs := map[int]string{}
	if cards, err := readFile(filepath.Join(procPath, "cards")); err == nil {
		cardIDs = parseALSACardIDs(cards)
	}

	pcm, err := readFile(filepath.Join(procPath, "pcm"))
	if err != nil {
		return nil, nil, fmt.Errorf("read ALSA PCM devices: %w", err)
	}

	inputDevices, outputDevices := parseALSAPCMDevices(pcm, cardIDs)
	return pcmDeviceNames(inputDevices), pcmDeviceNames(outputDevices), nil
}

func parseALSACardIDs(cards []byte) map[int]string {
	cardIDs := map[int]string{}
	for _, line := range strings.Split(string(cards), "\n") {
		openBracket := strings.IndexByte(line, '[')
		closeBracket := strings.IndexByte(line, ']')
		if openBracket < 0 || closeBracket <= openBracket {
			continue
		}

		cardNumber, err := strconv.Atoi(strings.TrimSpace(line[:openBracket]))
		if err != nil {
			continue
		}
		if cardID := strings.TrimSpace(line[openBracket+1 : closeBracket]); cardID != "" {
			cardIDs[cardNumber] = cardID
		}
	}
	return cardIDs
}

func parseALSAPCMDevices(pcm []byte, cardIDs map[int]string) ([]alsaPCMDevice, []alsaPCMDevice) {
	var inputDevices, outputDevices []alsaPCMDevice
	for _, line := range strings.Split(string(pcm), "\n") {
		header, details, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		cardNumber, deviceNumber, ok := parseALSAPCMHeader(header)
		if !ok {
			continue
		}

		device := alsaPCMDevice{
			cardNumber:   cardNumber,
			deviceNumber: deviceNumber,
			cardID:       cardIDs[cardNumber],
		}
		if alsaPCMHasStream(details, "capture") {
			inputDevices = append(inputDevices, device)
		}
		if alsaPCMHasStream(details, "playback") {
			outputDevices = append(outputDevices, device)
		}
	}
	sortPCMDevices(inputDevices)
	sortPCMDevices(outputDevices)
	return inputDevices, outputDevices
}

func parseALSAPCMHeader(header string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(header), "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	cardNumber, cardErr := strconv.Atoi(parts[0])
	deviceNumber, deviceErr := strconv.Atoi(parts[1])
	if cardErr != nil || deviceErr != nil {
		return 0, 0, false
	}
	return cardNumber, deviceNumber, true
}

func alsaPCMHasStream(details string, stream string) bool {
	for _, field := range strings.Split(details, ":") {
		parts := strings.Fields(field)
		if len(parts) < 2 || parts[0] != stream {
			continue
		}
		count, err := strconv.Atoi(parts[1])
		return err == nil && count > 0
	}
	return false
}

func sortPCMDevices(devices []alsaPCMDevice) {
	sort.Slice(devices, func(left int, right int) bool {
		if devices[left].cardNumber != devices[right].cardNumber {
			return devices[left].cardNumber < devices[right].cardNumber
		}
		return devices[left].deviceNumber < devices[right].deviceNumber
	})
}

func pcmDeviceNames(devices []alsaPCMDevice) []string {
	names := make([]string, len(devices))
	for index, device := range devices {
		if device.cardID == "" {
			names[index] = fmt.Sprintf("plughw:%d,%d", device.cardNumber, device.deviceNumber)
			continue
		}
		names[index] = fmt.Sprintf("plughw:CARD=%s,DEV=%d", device.cardID, device.deviceNumber)
	}
	return names
}

func printALSAPCMDevices(writer io.Writer, inputDevices []string, outputDevices []string) {
	printALSAPCMDeviceList(writer, "input", inputDevices)
	printALSAPCMDeviceList(writer, "output", outputDevices)
}

func printALSAPCMDeviceList(writer io.Writer, direction string, devices []string) {
	fmt.Fprintf(writer, "Available %s devices:\n", direction)
	if len(devices) == 0 {
		fmt.Fprintln(writer, "(none)")
		return
	}
	for index, device := range devices {
		fmt.Fprintf(writer, "[%d] %q\n", index+1, device)
	}
}
