package main

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDecodePCM16(t *testing.T) {
	raw := []byte{0x34, 0x12, 0x00, 0x80, 0xff, 0x7f}
	want := []int16{0x1234, -32768, 32767}
	if got := decodePCM16(raw); !reflect.DeepEqual(got, want) {
		t.Fatalf("decodePCM16() = %v, want %v", got, want)
	}
}

func TestPrintALSAPCMDevices(t *testing.T) {
	var output bytes.Buffer
	printALSAPCMDevices(&output, []string{"plughw:CARD=Device,DEV=0"}, []string{"plughw:CARD=Device,DEV=0"})
	const want = "Available input devices:\n[1] \"plughw:CARD=Device,DEV=0\"\nAvailable output devices:\n[1] \"plughw:CARD=Device,DEV=0\"\n"
	if got := output.String(); got != want {
		t.Fatalf("device list output = %q, want %q", got, want)
	}
}

func TestDiscoverALSAPCMDeviceNames(t *testing.T) {
	files := map[string]string{
		filepath.Join(alsaProcPath, "cards"): " 0 [PCH            ]: HDA-Intel - HDA Intel PCH\n 1 [Device         ]: USB-Audio - USB Audio Device\n",
		filepath.Join(alsaProcPath, "pcm"):   "00-00: ALC Audio : ALC Audio : playback 1\n01-00: USB Audio : USB Audio : playback 1 : capture 1\n01-01: USB Audio #1 : USB Audio #1 : capture 0\n",
	}
	inputDevices, outputDevices, err := discoverALSAPCMDeviceNames(alsaProcPath, func(path string) ([]byte, error) {
		contents, ok := files[path]
		if !ok {
			return nil, errors.New("unexpected ALSA procfs path")
		}
		return []byte(contents), nil
	})
	if err != nil {
		t.Fatalf("discoverALSAPCMDeviceNames() error = %v", err)
	}
	if got, want := inputDevices, []string{"plughw:CARD=Device,DEV=0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("input devices = %v, want %v", got, want)
	}
	if got, want := outputDevices, []string{"plughw:CARD=PCH,DEV=0", "plughw:CARD=Device,DEV=0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("output devices = %v, want %v", got, want)
	}
}

func TestCaptureAudioErrorPrefersArecordExit(t *testing.T) {
	waitErr := errors.New("exit status 1")
	err := captureAudioError("default", io.EOF, waitErr)
	if !errors.Is(err, waitErr) {
		t.Fatalf("captureAudioError() = %v, want wrapped process error %v", err, waitErr)
	}
	if got, want := err.Error(), "arecord failed for InputDevice \"default\": exit status 1"; got != want {
		t.Fatalf("captureAudioError() = %q, want %q", got, want)
	}
}

func TestGPIOWithoutRaspberryPiUsesNoOps(t *testing.T) {
	squelch, err := newSquelchInput(17, true, false)
	if err != nil {
		t.Fatalf("newSquelchInput() error = %v", err)
	}
	if squelch.enabled {
		t.Fatal("non-Raspberry Pi squelch should be disabled")
	}
	open, err := squelch.IsOpen()
	if err != nil {
		t.Fatalf("no-op squelch IsOpen() error = %v", err)
	}
	if !open {
		t.Fatal("no-op squelch should keep interface audio enabled")
	}
	if err := squelch.Close(); err != nil {
		t.Fatalf("no-op squelch Close() error = %v", err)
	}

	ptt, err := newGPIOOutput(4, true, false)
	if err != nil {
		t.Fatalf("newGPIOOutput() error = %v", err)
	}
	if ptt.enabled {
		t.Fatal("non-Raspberry Pi PTT should be disabled")
	}
	if err := ptt.Set(true); err != nil {
		t.Fatalf("no-op PTT Set() error = %v", err)
	}
	if err := ptt.Close(); err != nil {
		t.Fatalf("no-op PTT Close() error = %v", err)
	}
}
