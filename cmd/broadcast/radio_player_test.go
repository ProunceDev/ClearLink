package main

import (
	"clearlink/internal/network"
	"testing"
	"time"
)

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

func TestApplyAudioOutputGain(t *testing.T) {
	chunk := &network.ToAnyAudioChunkPacket{
		ChunkNumber: 7,
		SampleRate:  48000,
		Samples:     []int16{-20000, -2, 0, 3, 20000},
	}

	scaled := applyAudioOutputGain(chunk, 2)
	want := []int16{-32768, -4, 0, 6, 32767}
	if len(scaled.Samples) != len(want) {
		t.Fatalf("scaled sample count = %d, want %d", len(scaled.Samples), len(want))
	}
	for index, sample := range want {
		if got := scaled.Samples[index]; got != sample {
			t.Errorf("scaled sample %d = %d, want %d", index, got, sample)
		}
	}
	if got, want := chunk.Samples[0], int16(-20000); got != want {
		t.Errorf("source sample = %d, want %d", got, want)
	}
}

func TestPTTHangTimeExpired(t *testing.T) {
	lastAudioAt := time.Date(2026, time.November, 3, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name     string
		now      time.Time
		hangTime time.Duration
		want     bool
	}{
		{name: "before hang time", now: lastAudioAt.Add(49 * time.Millisecond), hangTime: 50 * time.Millisecond, want: false},
		{name: "at hang time", now: lastAudioAt.Add(50 * time.Millisecond), hangTime: 50 * time.Millisecond, want: true},
		{name: "zero hang time", now: lastAudioAt, hangTime: 0, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := pttHangTimeExpired(lastAudioAt, test.now, test.hangTime); got != test.want {
				t.Errorf("pttHangTimeExpired() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestStartPTTHangRestartsTimer(t *testing.T) {
	player := &RadioPlayer{
		transmitting: true,
		lastAudioAt:  time.Unix(0, 0),
		pttHangTime:  50 * time.Millisecond,
	}
	before := time.Now()
	player.startPTTHang()

	player.mu.Lock()
	lastAudioAt := player.lastAudioAt
	player.mu.Unlock()
	if lastAudioAt.Before(before) {
		t.Fatalf("last audio time = %v, want at or after %v", lastAudioAt, before)
	}
}
