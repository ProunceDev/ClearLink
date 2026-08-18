package sdrhelper

import (
	"clearlink/internal/models"
	"math"
	"testing"
)

func TestNarrowFMSquelchOpensAndCloses(t *testing.T) {
	squelch, err := newNarrowFMSquelch(16000, -20, 9.54, 0)
	if err != nil {
		t.Fatalf("newNarrowFMSquelch() error = %v", err)
	}

	for sample := 0; sample < 1000; sample++ {
		squelch.processRawSample(0.01)
	}
	for sample := 0; sample < 1000 && !squelch.isOpen(); sample++ {
		squelch.processRawSample(0.5)
	}
	if !squelch.isOpen() {
		t.Fatal("squelch did not open for a signal above the manual threshold")
	}

	for sample := 0; sample < 1000 && squelch.isOpen(); sample++ {
		squelch.processRawSample(0.01)
	}
	if squelch.isOpen() {
		t.Fatal("squelch did not close after the signal dropped")
	}
}

func TestNarrowFMSquelchCanBeAlwaysOpen(t *testing.T) {
	squelch, err := newNarrowFMSquelch(16000, 0, 0, 0)
	if err != nil {
		t.Fatalf("newNarrowFMSquelch() error = %v", err)
	}
	squelch.processRawSample(0.001)
	if !squelch.isOpen() || !squelch.shouldProcessAudio() {
		t.Fatal("zero SNR threshold should leave the squelch open")
	}
}

func TestNarrowFMProcessorSquelchUsesChannelFilteredIQ(t *testing.T) {
	processor, err := newNarrowFMProcessor(narrowFMSettings{
		InputSampleRate:      48000,
		OutputSampleRate:     48000,
		TargetFrequencyHz:    146520000,
		CenterFrequencyHz:    146520000,
		FMDeviationHz:        3000,
		ChannelBandwidthHz:   4000,
		SquelchThresholdDBFS: -30,
		AmpFactor:            1,
	})
	if err != nil {
		t.Fatalf("newNarrowFMProcessor() error = %v", err)
	}

	const (
		sampleRate = 48000.0
		interferer = 10000.0
		amplitude  = 0.25
	)
	for sample := 0; sample < int(sampleRate); sample++ {
		phase := 2 * math.Pi * interferer * float64(sample) / sampleRate
		processor.processIQ(amplitude*math.Cos(phase), amplitude*math.Sin(phase))
	}

	if got := processor.squelch.preFilter.full; got >= 0.02 {
		t.Fatalf("out-of-channel signal level = %f, want less than 0.02", got)
	}
	if got := processor.currentRSSIDBFS(); got >= -34 {
		t.Fatalf("out-of-channel RSSI = %f dBFS, want less than -34 dBFS", got)
	}
}

func TestNarrowFMSquelchRequiresConfiguredCTCSS(t *testing.T) {
	const sampleRate = 16000.0
	const targetTone = 100.0

	squelch, err := newNarrowFMSquelch(sampleRate, -20, 9.54, targetTone)
	if err != nil {
		t.Fatalf("newNarrowFMSquelch() error = %v", err)
	}
	for sample := 0; sample < 1000; sample++ {
		squelch.processRawSample(0.01)
	}
	for sample := 0; sample < 2000 && !squelch.isOpen(); sample++ {
		squelch.processRawSample(0.5)
		if squelch.shouldProcessAudio() {
			audio := math.Sin(2 * math.Pi * targetTone * float64(sample) / sampleRate)
			squelch.processAudioSample(audio)
		}
	}
	if !squelch.isOpen() {
		t.Fatal("squelch did not open after receiving the configured CTCSS tone")
	}
}

func TestCTCSSDetectorAcceptsTargetAndRejectsOtherTone(t *testing.T) {
	const sampleRate = 8000.0
	const targetTone = 100.0

	target, err := newCTCSSDetector(targetTone, sampleRate, 0.05*sampleRate)
	if err != nil {
		t.Fatalf("newCTCSSDetector() error = %v", err)
	}
	for sample := 0; sample < target.windowSize; sample++ {
		value := math.Sin(2 * math.Pi * targetTone * float64(sample) / sampleRate)
		target.process(value)
	}
	if !target.enoughSamples || !target.hasTone {
		t.Fatal("CTCSS detector did not identify the requested tone")
	}

	wrong, err := newCTCSSDetector(targetTone, sampleRate, 0.05*sampleRate)
	if err != nil {
		t.Fatalf("newCTCSSDetector() error = %v", err)
	}
	for sample := 0; sample < wrong.windowSize; sample++ {
		value := math.Sin(2 * math.Pi * 67.0 * float64(sample) / sampleRate)
		wrong.process(value)
	}
	if wrong.hasTone {
		t.Fatal("CTCSS detector accepted a different standard tone")
	}
}

func TestNotchFilterAttenuatesConfiguredTone(t *testing.T) {
	const sampleRate = 8000.0
	const notchFrequency = 1000.0
	notch := newNotchBiquad(notchFrequency, sampleRate, 10)

	inputEnergy := 0.0
	outputEnergy := 0.0
	for sample := 0; sample < 8000; sample++ {
		input := math.Sin(2 * math.Pi * notchFrequency * float64(sample) / sampleRate)
		output := notch.process(input)
		if sample >= 1000 {
			inputEnergy += input * input
			outputEnergy += output * output
		}
	}

	if math.Sqrt(outputEnergy/inputEnergy) > 0.1 {
		t.Fatalf("notch attenuation = %f, want less than 0.1", math.Sqrt(outputEnergy/inputEnergy))
	}
}

func TestNarrowFMProcessorProducesGatedAudio(t *testing.T) {
	processor, err := newNarrowFMProcessor(narrowFMSettings{
		InputSampleRate:      16000,
		OutputSampleRate:     8000,
		FMDeviationHz:        1000,
		LowpassHz:            3000,
		SquelchSNRThreshold:  0,
		DeemphasisTauSeconds: 0,
		AmpFactor:            1,
	})
	if err != nil {
		t.Fatalf("newNarrowFMProcessor() error = %v", err)
	}

	phase := 0.0
	outputEnergy := 0.0
	outputSamples := 0
	for sample := 0; sample < 32000; sample++ {
		deviation := 600 * math.Sin(2*math.Pi*500*float64(sample)/16000)
		phase += 2 * math.Pi * deviation / 16000
		audio, emitted, open := processor.processIQ(math.Cos(phase), math.Sin(phase))
		if emitted && sample > 8000 {
			if !open {
				t.Fatal("always-open processor unexpectedly gated audio")
			}
			outputEnergy += audio * audio
			outputSamples++
		}
	}

	if outputSamples == 0 || math.Sqrt(outputEnergy/float64(outputSamples)) < 0.05 {
		t.Fatal("NFM processor did not produce an audible demodulated signal")
	}
}

func TestValidateListenConfigRejectsInvalidDSPSettings(t *testing.T) {
	config := validListenTestConfig()
	if err := ValidateListenConfig(config); err != nil {
		t.Fatalf("ValidateListenConfig() error = %v", err)
	}

	for _, test := range []struct {
		name  string
		key   string
		value any
	}{
		{name: "zero audio rate", key: "AudioRate", value: 0},
		{name: "non-divisible sample rates", key: "SampleRate", value: 12000},
		{name: "sub-sample chunk duration", key: "AudioChunkMs", value: 0},
		{name: "oversized audio chunk", key: "AudioChunkMs", value: 10000},
		{name: "reversed output filters", key: "Lowpass", value: 50},
		{name: "invalid direct sampling", key: "DirectSampling", value: 3},
		{name: "negative center frequency", key: "CenterFrequency", value: -1},
		{name: "non-finite CTCSS", key: "CTCSS", value: math.Inf(1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := validListenTestConfig()
			for index := range candidate.Entries {
				if candidate.Entries[index].Key == test.key {
					candidate.Entries[index].Var.Data = test.value
					break
				}
			}
			if err := ValidateListenConfig(candidate); err == nil {
				t.Fatalf("ValidateListenConfig() accepted %s", test.name)
			}
		})
	}
}

func validListenTestConfig() models.Config {
	intEntry := func(key string, value int) models.ConfigEntry {
		return models.ConfigEntry{
			Key:  key,
			Type: models.ApplicationTypeListen,
			Var:  models.EntryVar{Type: models.EntryTypeInt, Data: value},
		}
	}
	floatEntry := func(key string, value float64) models.ConfigEntry {
		return models.ConfigEntry{
			Key:  key,
			Type: models.ApplicationTypeListen,
			Var:  models.EntryVar{Type: models.EntryTypeFloat, Data: value},
		}
	}

	return models.Config{Entries: []models.ConfigEntry{
		intEntry("SampleRate", 16000),
		intEntry("AudioRate", 8000),
		intEntry("FMDeviation", 1000),
		intEntry("Bandwidth", 0),
		intEntry("Highpass", 100),
		intEntry("Lowpass", 3000),
		intEntry("Tau", 0),
		intEntry("SquelchThreshold", -20),
		floatEntry("SquelchSNRThreshold", 9.54),
		floatEntry("CTCSS", 0),
		floatEntry("Notch", 0),
		floatEntry("NotchQ", 10),
		floatEntry("AmpFactor", 1),
		intEntry("DeviceIndex", 0),
		intEntry("DirectSampling", 0),
		intEntry("Frequency", 146520000),
		intEntry("CenterFrequency", 0),
		intEntry("AudioChunkMs", 20),
		intEntry("BufferSize", 16384),
		intEntry("AudioGain", 28000),
		intEntry("TunerBandwidth", 15000),
	}}
}

func TestNarrowFMProcessorFrequencyShiftRebasesCarrier(t *testing.T) {
	const inputSampleRate = 16000.0
	const targetFrequency = 146520000
	const centerFrequency = 146530000
	const offsetHz = centerFrequency - targetFrequency

	shifted, err := newNarrowFMProcessor(narrowFMSettings{
		InputSampleRate:      16000,
		OutputSampleRate:     8000,
		TargetFrequencyHz:    targetFrequency,
		CenterFrequencyHz:    centerFrequency,
		FMDeviationHz:        1000,
		LowpassHz:            3000,
		SquelchThresholdDBFS: 0,
		SquelchSNRThreshold:  0,
		AmpFactor:            1,
	})
	if err != nil {
		t.Fatalf("newNarrowFMProcessor() error = %v", err)
	}

	unshifted, err := newNarrowFMProcessor(narrowFMSettings{
		InputSampleRate:      16000,
		OutputSampleRate:     8000,
		TargetFrequencyHz:    targetFrequency,
		CenterFrequencyHz:    targetFrequency,
		FMDeviationHz:        1000,
		LowpassHz:            3000,
		SquelchThresholdDBFS: 0,
		SquelchSNRThreshold:  0,
		AmpFactor:            1,
	})
	if err != nil {
		t.Fatalf("newNarrowFMProcessor() error = %v", err)
	}

	shiftedEnergy := 0.0
	unshiftedEnergy := 0.0
	for sample := 0; sample < 4000; sample++ {
		phase := 2 * math.Pi * offsetHz * float64(sample) / inputSampleRate
		i := math.Cos(phase)
		q := math.Sin(phase)

		shiftedAudio, shiftedEmitted, shiftedOpen := shifted.processIQ(i, q)
		if shiftedEmitted && shiftedOpen {
			shiftedEnergy += shiftedAudio * shiftedAudio
		}

		unshiftedAudio, unshiftedEmitted, unshiftedOpen := unshifted.processIQ(i, q)
		if unshiftedEmitted && unshiftedOpen {
			unshiftedEnergy += unshiftedAudio * unshiftedAudio
		}
	}

	if shiftedEnergy >= unshiftedEnergy*0.25 {
		t.Fatalf("frequency shift did not sufficiently reduce demodulated energy: shifted=%f unshifted=%f", shiftedEnergy, unshiftedEnergy)
	}
}

func TestNarrowFMProcessorKeepsNegativeFrequencyShiftPhaseBounded(t *testing.T) {
	processor, err := newNarrowFMProcessor(narrowFMSettings{
		InputSampleRate:      48000,
		OutputSampleRate:     48000,
		TargetFrequencyHz:    146520000,
		CenterFrequencyHz:    146510000,
		FMDeviationHz:        3000,
		SquelchSNRThreshold:  0,
		AmpFactor:            1,
	})
	if err != nil {
		t.Fatalf("newNarrowFMProcessor() error = %v", err)
	}

	for sample := 0; sample < 1000; sample++ {
		processor.processIQ(0, 0)
	}

	if math.Abs(processor.frequencyShiftPhase) > math.Pi+1e-12 {
		t.Fatalf("negative frequency-shift phase = %f, want it within [-pi, pi]", processor.frequencyShiftPhase)
	}
}
