package main

import "testing"

func TestNewGPIOPTTWithoutRaspberryPiUsesNoOp(t *testing.T) {
	ptt, err := newGPIOPTT(4, false)
	if err != nil {
		t.Fatalf("newGPIOPTT() error = %v", err)
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
