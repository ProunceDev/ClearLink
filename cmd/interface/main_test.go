package main

import (
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
