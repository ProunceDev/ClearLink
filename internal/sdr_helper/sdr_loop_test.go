//go:build cgo

package sdrhelper

import "testing"

func TestNearestTunerGain(t *testing.T) {
	for _, test := range []struct {
		name      string
		gains     []int
		requested int
		want      int
	}{
		{name: "exact match", gains: []int{0, 90, 254}, requested: 254, want: 254},
		{name: "nearest match", gains: []int{0, 90, 140}, requested: 120, want: 140},
		{name: "tie keeps first gain", gains: []int{90, 110}, requested: 100, want: 90},
		{name: "empty gains preserves request", requested: 254, want: 254},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := nearestTunerGain(test.gains, test.requested); got != test.want {
				t.Fatalf("nearestTunerGain(%v, %d) = %d, want %d", test.gains, test.requested, got, test.want)
			}
		})
	}
}