package sdrhelper

import "math"

type biquad struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

func (filter *biquad) process(sample float64) float64 {
	output := filter.b0*sample + filter.b1*filter.x1 + filter.b2*filter.x2 - filter.a1*filter.y1 - filter.a2*filter.y2
	filter.x2, filter.x1 = filter.x1, sample
	filter.y2, filter.y1 = filter.y1, output
	return output
}

func polarDiscriminant(currentI, currentQ, previousI, previousQ float64) float64 {
	cross := currentQ*previousI - currentI*previousQ
	dot := currentI*previousI + currentQ*previousQ
	return math.Atan2(cross, dot)
}

func newLowpassBiquad(cutoffHz, sampleRate, qFactor float64) *biquad {
	w0 := 2 * math.Pi * cutoffHz / sampleRate
	cosw0 := math.Cos(w0)
	alpha := math.Sin(w0) / (2 * qFactor)
	a0 := 1 + alpha

	return &biquad{
		b0: ((1 - cosw0) / 2) / a0,
		b1: (1 - cosw0) / a0,
		b2: ((1 - cosw0) / 2) / a0,
		a1: (-2 * cosw0) / a0,
		a2: (1 - alpha) / a0,
	}
}

// SDRData holds receiver output for a single PCM chunk.
type SDRData struct {
	RSSI        float64 // Average signal strength in dBFS for this chunk
	SampleRate  int     // Sample rate for AudioChunk
	AudioChunk  []int16 // 16-bit PCM mono audio chunk
	SquelchOpen bool    // Squelch state at the end of this chunk
	HasAudio    bool    // At least one sample in this chunk passed the squelch gate
}
