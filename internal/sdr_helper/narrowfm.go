package sdrhelper

import (
	"clearlink/internal/models"
	"clearlink/internal/network"
	"fmt"
	"math"
)

const nfmReferenceSampleRate = 16000.0

type narrowFMSettings struct {
	InputSampleRate      int
	OutputSampleRate     int
	TargetFrequencyHz    int
	CenterFrequencyHz    int
	FMDeviationHz        float64
	ChannelBandwidthHz   float64
	HighpassHz           float64
	LowpassHz            float64
	DeemphasisTauSeconds float64
	SquelchThresholdDBFS float64
	SquelchSNRThreshold  float64
	CTCSSHz              float64
	NotchHz              float64
	NotchQ               float64
	AmpFactor            float64
}

func (settings narrowFMSettings) validate() error {
	if settings.InputSampleRate < 1 || settings.OutputSampleRate < 1 {
		return fmt.Errorf("sample rates must be positive")
	}
	if settings.TargetFrequencyHz <= 0 {
		return fmt.Errorf("frequency must be positive")
	}
	if settings.CenterFrequencyHz < 0 {
		return fmt.Errorf("center frequency must not be negative")
	}
	for _, value := range []struct {
		name string
		data float64
	}{
		{name: "FM deviation", data: settings.FMDeviationHz},
		{name: "channel bandwidth", data: settings.ChannelBandwidthHz},
		{name: "highpass frequency", data: settings.HighpassHz},
		{name: "lowpass frequency", data: settings.LowpassHz},
		{name: "de-emphasis tau", data: settings.DeemphasisTauSeconds},
		{name: "squelch threshold", data: settings.SquelchThresholdDBFS},
		{name: "squelch SNR threshold", data: settings.SquelchSNRThreshold},
		{name: "CTCSS frequency", data: settings.CTCSSHz},
		{name: "notch frequency", data: settings.NotchHz},
		{name: "notch Q", data: settings.NotchQ},
		{name: "amplification factor", data: settings.AmpFactor},
	} {
		if math.IsNaN(value.data) || math.IsInf(value.data, 0) {
			return fmt.Errorf("%s must be finite", value.name)
		}
	}
	if settings.InputSampleRate%settings.OutputSampleRate != 0 {
		return fmt.Errorf("input sample rate %d must be divisible by output sample rate %d", settings.InputSampleRate, settings.OutputSampleRate)
	}
	if settings.FMDeviationHz <= 0 {
		return fmt.Errorf("FM deviation must be positive")
	}
	if settings.ChannelBandwidthHz < 0 || settings.ChannelBandwidthHz >= float64(settings.InputSampleRate) {
		return fmt.Errorf("channel bandwidth must be between 0 and the input sample rate")
	}
	if settings.HighpassHz < 0 || settings.HighpassHz >= float64(settings.OutputSampleRate)/2 {
		return fmt.Errorf("highpass frequency must be below output Nyquist")
	}
	if settings.LowpassHz < 0 || settings.LowpassHz >= float64(settings.OutputSampleRate)/2 {
		return fmt.Errorf("lowpass frequency must be below output Nyquist")
	}
	if settings.LowpassHz > 0 && settings.LowpassHz < settings.HighpassHz {
		return fmt.Errorf("lowpass frequency must be greater than or equal to highpass frequency")
	}
	if settings.DeemphasisTauSeconds < 0 {
		return fmt.Errorf("de-emphasis tau must not be negative")
	}
	if settings.SquelchThresholdDBFS > 0 {
		return fmt.Errorf("squelch threshold must be less than or equal to 0 dBFS")
	}
	if settings.SquelchSNRThreshold < 0 {
		return fmt.Errorf("squelch SNR threshold must be non-negative")
	}
	if settings.CTCSSHz < 0 || settings.CTCSSHz >= float64(settings.OutputSampleRate)/2 {
		return fmt.Errorf("CTCSS frequency must be below output Nyquist")
	}
	if settings.NotchHz < 0 || settings.NotchHz >= float64(settings.OutputSampleRate)/2 {
		return fmt.Errorf("notch frequency must be below output Nyquist")
	}
	if settings.NotchHz > 0 && settings.NotchQ <= 0 {
		return fmt.Errorf("notch Q must be positive when a notch is configured")
	}
	if settings.AmpFactor < 0 {
		return fmt.Errorf("amplification factor must not be negative")
	}
	return nil
}

// ValidateListenConfig verifies that a listener configuration can construct a valid narrow-FM receiver.
func ValidateListenConfig(cfg models.Config) error {
	settings, err := narrowFMSettingsFromEntries(cfg.Entries)
	if err != nil {
		return err
	}
	if err := settings.validate(); err != nil {
		return err
	}

	deviceIndex, err := listenConfigIntFromEntries(cfg.Entries, "DeviceIndex")
	if err != nil {
		return err
	}
	if deviceIndex < 0 {
		return fmt.Errorf("device index must not be negative")
	}

	directSampling, err := listenConfigIntFromEntries(cfg.Entries, "DirectSampling")
	if err != nil {
		return err
	}
	if directSampling < 0 || directSampling > 2 {
		return fmt.Errorf("direct sampling must be 0, 1, or 2")
	}

	frequency, err := listenConfigIntFromEntries(cfg.Entries, "Frequency")
	if err != nil {
		return err
	}
	if frequency <= 0 {
		return fmt.Errorf("frequency must be positive")
	}

	centerFrequency, err := listenConfigIntFromEntries(cfg.Entries, "CenterFrequency")
	if err != nil {
		return err
	}
	if centerFrequency < 0 {
		return fmt.Errorf("center frequency must not be negative")
	}

	chunkDuration, err := listenConfigIntFromEntries(cfg.Entries, "AudioChunkMs")
	if err != nil {
		return err
	}
	if _, err := audioChunkSampleCount(settings.OutputSampleRate, chunkDuration); err != nil {
		return err
	}

	bufferSize, err := listenConfigIntFromEntries(cfg.Entries, "BufferSize")
	if err != nil {
		return err
	}
	if bufferSize < 2 {
		return fmt.Errorf("SDR buffer size must contain at least one IQ sample")
	}

	audioGain, err := listenConfigIntFromEntries(cfg.Entries, "AudioGain")
	if err != nil {
		return err
	}
	if audioGain < 0 {
		return fmt.Errorf("audio gain must not be negative")
	}

	tunerBandwidth, err := listenConfigIntFromEntries(cfg.Entries, "TunerBandwidth")
	if err != nil {
		return err
	}
	if tunerBandwidth < 0 {
		return fmt.Errorf("tuner bandwidth must not be negative")
	}

	return nil
}

func audioChunkSampleCount(sampleRate, durationMilliseconds int) (int, error) {
	if sampleRate <= 0 || durationMilliseconds <= 0 {
		return 0, fmt.Errorf("audio chunk duration and sample rate must be positive")
	}

	const maxInt64 = int64(^uint64(0) >> 1)
	sampleRate64 := int64(sampleRate)
	duration64 := int64(durationMilliseconds)
	if duration64 > maxInt64/sampleRate64 {
		return 0, fmt.Errorf("audio chunk duration is too large")
	}
	sampleCount := sampleRate64 * duration64 / 1000
	if sampleCount < 1 {
		return 0, fmt.Errorf("audio chunk duration produces fewer than one sample")
	}
	if sampleCount > int64(network.MaxAudioChunkSamples) {
		return 0, fmt.Errorf("audio chunk contains %d samples; maximum is %d", sampleCount, network.MaxAudioChunkSamples)
	}
	return int(sampleCount), nil
}

func narrowFMSettingsFromEntries(entries []models.ConfigEntry) (narrowFMSettings, error) {
	settings := narrowFMSettings{}
	var err error

	if settings.InputSampleRate, err = listenConfigIntFromEntries(entries, "SampleRate"); err != nil {
		return narrowFMSettings{}, err
	}
	if settings.OutputSampleRate, err = listenConfigIntFromEntries(entries, "AudioRate"); err != nil {
		return narrowFMSettings{}, err
	}
	if settings.TargetFrequencyHz, err = listenConfigIntFromEntries(entries, "Frequency"); err != nil {
		return narrowFMSettings{}, err
	}
	if settings.CenterFrequencyHz, err = listenConfigIntFromEntries(entries, "CenterFrequency"); err != nil {
		return narrowFMSettings{}, err
	}
	if settings.FMDeviationHz, err = listenConfigFloatFromIntEntries(entries, "FMDeviation"); err != nil {
		return narrowFMSettings{}, err
	}
	if settings.ChannelBandwidthHz, err = listenConfigFloatFromIntEntries(entries, "Bandwidth"); err != nil {
		return narrowFMSettings{}, err
	}
	if settings.HighpassHz, err = listenConfigFloatFromIntEntries(entries, "Highpass"); err != nil {
		return narrowFMSettings{}, err
	}
	if settings.LowpassHz, err = listenConfigFloatFromIntEntries(entries, "Lowpass"); err != nil {
		return narrowFMSettings{}, err
	}

	tauMicroseconds, err := listenConfigIntFromEntries(entries, "Tau")
	if err != nil {
		return narrowFMSettings{}, err
	}
	settings.DeemphasisTauSeconds = float64(tauMicroseconds) * 1e-6

	if settings.SquelchThresholdDBFS, err = listenConfigFloatFromIntEntries(entries, "SquelchThreshold"); err != nil {
		return narrowFMSettings{}, err
	}
	if settings.SquelchSNRThreshold, err = listenConfigFloatFromEntries(entries, "SquelchSNRThreshold"); err != nil {
		return narrowFMSettings{}, err
	}
	if settings.CTCSSHz, err = listenConfigFloatFromEntries(entries, "CTCSS"); err != nil {
		return narrowFMSettings{}, err
	}
	if settings.NotchHz, err = listenConfigFloatFromEntries(entries, "Notch"); err != nil {
		return narrowFMSettings{}, err
	}
	if settings.NotchQ, err = listenConfigFloatFromEntries(entries, "NotchQ"); err != nil {
		return narrowFMSettings{}, err
	}
	if settings.AmpFactor, err = listenConfigFloatFromEntries(entries, "AmpFactor"); err != nil {
		return narrowFMSettings{}, err
	}

	return settings, nil
}

func listenConfigIntFromEntries(entries []models.ConfigEntry, key string) (int, error) {
	for _, entry := range entries {
		if entry.Type != models.ApplicationTypeListen || entry.Key != key {
			continue
		}
		value, ok := entry.Var.Data.(int)
		if !ok {
			return 0, fmt.Errorf("listen config %s must be an integer", key)
		}
		return value, nil
	}
	return 0, fmt.Errorf("listen config %s is missing", key)
}

func listenConfigFloatFromIntEntries(entries []models.ConfigEntry, key string) (float64, error) {
	value, err := listenConfigIntFromEntries(entries, key)
	if err != nil {
		return 0, err
	}
	return float64(value), nil
}

func listenConfigFloatFromEntries(entries []models.ConfigEntry, key string) (float64, error) {
	for _, entry := range entries {
		if entry.Type != models.ApplicationTypeListen || entry.Key != key {
			continue
		}
		value, ok := entry.Var.Data.(float64)
		if !ok {
			return 0, fmt.Errorf("listen config %s must be a floating-point value", key)
		}
		return value, nil
	}
	return 0, fmt.Errorf("listen config %s is missing", key)
}

type narrowFMProcessor struct {
	settings            narrowFMSettings
	decimation          int
	decimationCounter   int
	demodScale          float64
	frequencyShiftStep  float64
	frequencyShiftPhase float64
	previousI           float64
	previousQ           float64
	hasPreviousIQ       bool
	iqFilterI           *biquad
	iqFilterQ           *biquad
	discriminatorFilter []*biquad
	dcBlocker           dcBlocker
	deemphasis          deemphasisFilter
	highpass            *biquad
	lowpass             *biquad
	notch               *biquad
	squelch             *narrowFMSquelch
}

func newNarrowFMProcessor(settings narrowFMSettings) (*narrowFMProcessor, error) {
	if err := settings.validate(); err != nil {
		return nil, err
	}

	outputRate := float64(settings.OutputSampleRate)
	inputRate := float64(settings.InputSampleRate)
	processor := &narrowFMProcessor{
		settings:   settings,
		decimation: settings.InputSampleRate / settings.OutputSampleRate,
		demodScale: inputRate / (2 * math.Pi * settings.FMDeviationHz),
		frequencyShiftStep: 2 * math.Pi * float64(settings.CenterFrequencyHz-settings.TargetFrequencyHz) / inputRate,
		discriminatorFilter: []*biquad{
			newLowpassBiquad(antiAliasCutoff(settings.LowpassHz, outputRate), inputRate, 0.5412),
			newLowpassBiquad(antiAliasCutoff(settings.LowpassHz, outputRate), inputRate, 1.3065),
		},
		deemphasis: newDeemphasisFilter(outputRate, settings.DeemphasisTauSeconds),
	}

	if settings.ChannelBandwidthHz > 0 {
		cutoff := settings.ChannelBandwidthHz / 2
		processor.iqFilterI = newBesselLowpassBiquad(cutoff, inputRate)
		processor.iqFilterQ = newBesselLowpassBiquad(cutoff, inputRate)
	}
	if settings.HighpassHz > 0 {
		processor.highpass = newHighpassBiquad(settings.HighpassHz, outputRate, 1/math.Sqrt2)
	}
	if settings.LowpassHz > 0 {
		processor.lowpass = newLowpassBiquad(settings.LowpassHz, outputRate, 1/math.Sqrt2)
	}
	if settings.NotchHz > 0 {
		processor.notch = newNotchBiquad(settings.NotchHz, outputRate, settings.NotchQ)
	}

	squelch, err := newNarrowFMSquelch(
		outputRate,
		settings.SquelchThresholdDBFS,
		settings.SquelchSNRThreshold,
		settings.CTCSSHz,
	)
	if err != nil {
		return nil, err
	}
	processor.squelch = squelch

	return processor, nil
}

func antiAliasCutoff(lowpassHz, outputRate float64) float64 {
	maximum := outputRate * 0.45
	if lowpassHz <= 0 || lowpassHz > maximum {
		return maximum
	}
	return lowpassHz
}

func (processor *narrowFMProcessor) processIQ(rawI, rawQ float64) (float64, bool, bool) {
	if processor.frequencyShiftStep != 0 {
		sinPhase, cosPhase := math.Sincos(processor.frequencyShiftPhase)
		rawI, rawQ = rawI*cosPhase-rawQ*sinPhase, rawI*sinPhase+rawQ*cosPhase
		processor.frequencyShiftPhase += processor.frequencyShiftStep
		if processor.frequencyShiftPhase > math.Pi || processor.frequencyShiftPhase < -math.Pi {
			processor.frequencyShiftPhase = math.Remainder(processor.frequencyShiftPhase, 2*math.Pi)
		}
	}

	filteredI, filteredQ := rawI, rawQ
	if processor.iqFilterI != nil {
		filteredI = processor.iqFilterI.process(rawI)
		filteredQ = processor.iqFilterQ.process(rawQ)
	}
	channelLevel := math.Hypot(filteredI, filteredQ)

	if !processor.hasPreviousIQ {
		processor.previousI = filteredI
		processor.previousQ = filteredQ
		processor.hasPreviousIQ = true
		return 0, false, false
	}

	demodulated := polarDiscriminant(filteredI, filteredQ, processor.previousI, processor.previousQ)
	processor.previousI = filteredI
	processor.previousQ = filteredQ
	demodulated *= processor.demodScale
	for _, filter := range processor.discriminatorFilter {
		demodulated = filter.process(demodulated)
	}

	processor.decimationCounter++
	if processor.decimationCounter < processor.decimation {
		return 0, false, false
	}
	processor.decimationCounter = 0

	processor.squelch.processRawSample(channelLevel)
	if processor.iqFilterI != nil && processor.squelch.shouldFilterSample() {
		processor.squelch.processFilteredSample(channelLevel)
	}
	if !processor.squelch.shouldProcessAudio() {
		return 0, true, false
	}

	audio := processor.dcBlocker.process(demodulated)
	audio = processor.deemphasis.process(audio)
	processor.squelch.processAudioSample(audio)
	if !processor.squelch.isOpen() {
		return 0, true, false
	}

	if processor.notch != nil {
		audio = processor.notch.process(audio)
	}
	audio *= processor.settings.AmpFactor
	if processor.highpass != nil {
		audio = processor.highpass.process(audio)
	}
	if processor.lowpass != nil {
		audio = processor.lowpass.process(audio)
	}

	return clampAudio(audio), true, true
}

func (processor *narrowFMProcessor) currentSNRDB() float64 {
	if processor.squelch == nil {
		return -200
	}
	return processor.squelch.currentSNRDB()
}

func (processor *narrowFMProcessor) currentRSSIDBFS() float64 {
	if processor.squelch == nil {
		return -200
	}
	return processor.squelch.currentSignalDBFS()
}

func clampAudio(sample float64) float64 {
	if math.IsNaN(sample) {
		return 0
	}
	if sample > 1 {
		return 1
	}
	if sample < -1 {
		return -1
	}
	return sample
}

func newHighpassBiquad(cutoffHz, sampleRate, qFactor float64) *biquad {
	w0 := 2 * math.Pi * cutoffHz / sampleRate
	cosw0 := math.Cos(w0)
	alpha := math.Sin(w0) / (2 * qFactor)
	a0 := 1 + alpha

	return &biquad{
		b0: ((1 + cosw0) / 2) / a0,
		b1: (-(1 + cosw0)) / a0,
		b2: ((1 + cosw0) / 2) / a0,
		a1: (-2 * cosw0) / a0,
		a2: (1 - alpha) / a0,
	}
}

func newNotchBiquad(notchHz, sampleRate, qFactor float64) *biquad {
	w0 := 2 * math.Pi * notchHz / sampleRate
	cosw0 := math.Cos(w0)
	alpha := math.Sin(w0) / (2 * qFactor)
	a0 := 1 + alpha

	return &biquad{
		b0: 1 / a0,
		b1: (-2 * cosw0) / a0,
		b2: 1 / a0,
		a1: (-2 * cosw0) / a0,
		a2: (1 - alpha) / a0,
	}
}

func newBesselLowpassBiquad(cutoffHz, sampleRate float64) *biquad {
	warpedCutoff := math.Tan(math.Pi*cutoffHz/sampleRate) / math.Pi
	analogReal := 2 * math.Pi * warpedCutoff * -1.10160133059
	analogImaginary := 2 * math.Pi * warpedCutoff * 0.636009824757
	denominator := (2-analogReal)*(2-analogReal) + analogImaginary*analogImaginary
	poleReal := ((2+analogReal)*(2-analogReal) - analogImaginary*analogImaginary) / denominator
	poleImaginary := 4 * analogImaginary / denominator
	a1 := -2 * poleReal
	a2 := poleReal*poleReal + poleImaginary*poleImaginary
	gain := 4 / (1 + a1 + a2)

	return &biquad{
		b0: 1 / gain,
		b1: 2 / gain,
		b2: 1 / gain,
		a1: a1,
		a2: a2,
	}
}

type dcBlocker struct {
	average float64
}

func (blocker *dcBlocker) process(sample float64) float64 {
	blocker.average = blocker.average*0.995 + sample*0.005
	return sample - blocker.average
}

type deemphasisFilter struct {
	alpha float64
	value float64
}

func newDeemphasisFilter(sampleRate, tauSeconds float64) deemphasisFilter {
	if tauSeconds <= 0 {
		return deemphasisFilter{}
	}
	return deemphasisFilter{alpha: math.Exp(-1 / (sampleRate * tauSeconds))}
}

func (filter *deemphasisFilter) process(sample float64) float64 {
	filter.value = sample*(1-filter.alpha) + filter.value*filter.alpha
	return filter.value
}

type squelchPhase uint8

const (
	squelchClosed squelchPhase = iota
	squelchOpening
	squelchClosing
	squelchLowSignalAbort
	squelchOpen
)

type movingAverage struct {
	full   float64
	capped float64
}

type narrowFMSquelch struct {
	noiseFloor           float64
	manualThreshold      float64
	usingManualThreshold bool
	alwaysOpen           bool
	normalSignalRatio    float64
	flappySignalRatio    float64
	movingAverageCap     float64
	preFilter            movingAverage
	postFilter           movingAverage
	usingPostFilter      bool
	preVsPostFactor      float64
	openDelay            int
	closeDelay           int
	lowSignalAbort       int
	currentPhase         squelchPhase
	nextPhase            squelchPhase
	delay                int
	openCount            int
	sampleCount          int
	flappyCount          int
	lowSignalCount       int
	recentSampleSize     int
	flapOpensThreshold   int
	recentOpenCount      int
	closedSampleCount    int
	buffer               []float64
	bufferHead           int
	bufferTail           int
	ctcssFast            *ctcssDetector
	ctcssSlow            *ctcssDetector
}

func newNarrowFMSquelch(sampleRate, manualThresholdDBFS, snrThresholdDB, ctcssHz float64) (*narrowFMSquelch, error) {
	if sampleRate <= 0 {
		return nil, fmt.Errorf("squelch sample rate must be positive")
	}
	if manualThresholdDBFS > 0 {
		return nil, fmt.Errorf("manual squelch threshold must be less than or equal to 0 dBFS")
	}
	if snrThresholdDB < 0 {
		return nil, fmt.Errorf("squelch SNR threshold must be non-negative")
	}

	bufferSize := scaledSquelchSamples(102, sampleRate)
	squelch := &narrowFMSquelch{
		noiseFloor:           5,
		manualThreshold:      dbfsToLevel(manualThresholdDBFS),
		usingManualThreshold: manualThresholdDBFS < 0,
		alwaysOpen:           manualThresholdDBFS == 0 && snrThresholdDB == 0,
		normalSignalRatio:    math.Pow(10, snrThresholdDB/20),
		preFilter:            movingAverage{full: 0.001, capped: 0.001},
		postFilter:           movingAverage{full: 0.001, capped: 0.001},
		preVsPostFactor:      0.9,
		openDelay:            scaledSquelchSamples(197, sampleRate),
		closeDelay:           scaledSquelchSamples(197, sampleRate),
		lowSignalAbort:       scaledSquelchSamples(88, sampleRate),
		currentPhase:         squelchClosed,
		nextPhase:            squelchClosed,
		recentSampleSize:     scaledSquelchSamples(1000, sampleRate),
		flapOpensThreshold:   3,
		buffer:               make([]float64, bufferSize),
		bufferTail:           1 % bufferSize,
	}
	squelch.flappySignalRatio = squelch.normalSignalRatio * 0.9
	squelch.recalculateMovingAverageCap()
	if squelch.alwaysOpen {
		squelch.currentPhase = squelchOpen
		squelch.nextPhase = squelchOpen
	}

	if ctcssHz > 0 {
		fast, err := newCTCSSDetector(ctcssHz, sampleRate, sampleRate*0.05)
		if err != nil {
			return nil, err
		}
		slow, err := newCTCSSDetector(ctcssHz, sampleRate, sampleRate*0.4)
		if err != nil {
			return nil, err
		}
		squelch.ctcssFast = fast
		squelch.ctcssSlow = slow
	}

	return squelch, nil
}

func scaledSquelchSamples(referenceSamples int, sampleRate float64) int {
	samples := int(math.Round(float64(referenceSamples) * sampleRate / nfmReferenceSampleRate))
	if samples < 1 {
		return 1
	}
	return samples
}

func dbfsToLevel(decibels float64) float64 {
	return math.Pow(10, decibels/20)
}

func (squelch *narrowFMSquelch) processRawSample(sample float64) {
	squelch.advanceState()
	squelch.sampleCount++
	if squelch.sampleCount%16 == 0 {
		squelch.updateNoiseFloor()
	}

	squelch.updateMovingAverage(&squelch.preFilter, sample)
	squelch.buffer[squelch.bufferHead] = squelch.preFilter.capped * squelch.preVsPostFactor

	if squelch.alwaysOpen {
		return
	}
	if squelch.currentPhase == squelchOpen && !squelch.hasSignal() {
		squelch.requestPhase(squelchClosing)
	}
	if squelch.currentPhase == squelchClosed && squelch.hasSignal() {
		squelch.requestPhase(squelchOpening)
	}
	if squelch.currentPhase != squelchClosed && squelch.currentPhase != squelchLowSignalAbort {
		if sample >= squelch.level() {
			squelch.lowSignalCount = 0
		} else {
			squelch.lowSignalCount++
			if squelch.lowSignalCount >= squelch.lowSignalAbort {
				squelch.requestPhase(squelchLowSignalAbort)
			}
		}
	}
}

func (squelch *narrowFMSquelch) processFilteredSample(sample float64) {
	if !squelch.shouldFilterSample() {
		return
	}
	if squelch.currentPhase == squelchOpening {
		if squelch.delay < len(squelch.buffer) {
			return
		}
		if squelch.delay == len(squelch.buffer) {
			value := squelch.buffer[squelch.bufferTail]
			squelch.postFilter = movingAverage{full: value, capped: value}
		}
	}

	squelch.usingPostFilter = true
	squelch.updateMovingAverage(&squelch.postFilter, sample)
	if squelch.postFilter.capped < squelch.buffer[squelch.bufferTail] {
		squelch.requestPhase(squelchClosed)
	}
}

func (squelch *narrowFMSquelch) processAudioSample(sample float64) {
	if squelch.ctcssSlow == nil || squelch.currentPhase == squelchClosed {
		return
	}
	squelch.ctcssSlow.process(sample)
	if !squelch.ctcssSlow.enoughSamples {
		squelch.ctcssFast.process(sample)
	}
}

func (squelch *narrowFMSquelch) isOpen() bool {
	if squelch.currentPhase != squelchOpen && squelch.currentPhase != squelchClosing {
		return false
	}
	if squelch.ctcssSlow == nil {
		return true
	}
	if squelch.ctcssSlow.enoughSamples {
		return squelch.ctcssSlow.hasTone
	}
	return squelch.ctcssFast.hasTone
}

func (squelch *narrowFMSquelch) shouldFilterSample() bool {
	return (squelch.hasPreFilterSignal() || squelch.currentPhase != squelchClosed) && squelch.currentPhase != squelchLowSignalAbort
}

func (squelch *narrowFMSquelch) shouldProcessAudio() bool {
	return squelch.currentPhase == squelchOpen || squelch.currentPhase == squelchClosing
}

func (squelch *narrowFMSquelch) advanceState() {
	switch squelch.nextPhase {
	case squelchOpening:
		if squelch.currentPhase != squelchOpening {
			squelch.delay = 0
			squelch.lowSignalCount = 0
			squelch.usingPostFilter = false
			squelch.currentPhase = squelchOpening
		} else {
			squelch.delay++
			if squelch.delay >= squelch.openDelay {
				if squelch.closedSampleCount < squelch.recentSampleSize {
					squelch.recentOpenCount++
					if squelch.isFlapping() {
						squelch.flappyCount++
					}
				}
				if squelch.hasSignal() {
					squelch.nextPhase = squelchOpen
				} else {
					squelch.nextPhase = squelchClosed
				}
			}
		}
	case squelchClosing:
		if squelch.currentPhase != squelchClosing {
			squelch.delay = 0
			squelch.currentPhase = squelchClosing
		} else {
			squelch.delay++
			if squelch.delay >= squelch.closeDelay {
				if squelch.hasSignal() {
					squelch.currentPhase = squelchOpen
					squelch.nextPhase = squelchOpen
				} else {
					squelch.nextPhase = squelchClosed
				}
			}
		}
	case squelchLowSignalAbort:
		if squelch.currentPhase != squelchLowSignalAbort {
			if squelch.currentPhase != squelchClosing {
				squelch.delay = 0
			}
			squelch.currentPhase = squelchLowSignalAbort
		} else {
			squelch.delay++
			if squelch.delay >= squelch.closeDelay {
				squelch.nextPhase = squelchClosed
			}
		}
	case squelchOpen:
		if squelch.currentPhase != squelchOpen {
			squelch.openCount++
			squelch.currentPhase = squelchOpen
		}
	case squelchClosed:
		if squelch.currentPhase != squelchClosed {
			squelch.usingPostFilter = false
			squelch.closedSampleCount = 0
			squelch.currentPhase = squelchClosed
			if squelch.ctcssFast != nil {
				squelch.ctcssFast.reset()
				squelch.ctcssSlow.reset()
			}
		} else if squelch.closedSampleCount < squelch.recentSampleSize {
			squelch.closedSampleCount++
		} else if squelch.closedSampleCount == squelch.recentSampleSize {
			squelch.recentOpenCount = 0
		}
	}

	squelch.bufferTail = (squelch.bufferTail + 1) % len(squelch.buffer)
	squelch.bufferHead = (squelch.bufferHead + 1) % len(squelch.buffer)
}

func (squelch *narrowFMSquelch) requestPhase(next squelchPhase) {
	switch squelch.currentPhase {
	case squelchClosed:
		if next == squelchOpening || next == squelchOpen {
			squelch.nextPhase = squelchOpening
		} else {
			squelch.nextPhase = squelchClosed
		}
	case squelchOpening:
		if next == squelchLowSignalAbort {
			squelch.nextPhase = squelchClosed
		} else {
			squelch.nextPhase = next
		}
	case squelchLowSignalAbort:
		if next == squelchLowSignalAbort || next == squelchClosed {
			squelch.nextPhase = next
		} else {
			squelch.nextPhase = squelchClosed
		}
	case squelchOpen:
		if next == squelchClosed {
			squelch.nextPhase = squelchClosing
		} else if next == squelchOpening {
			squelch.nextPhase = squelchOpen
		} else {
			squelch.nextPhase = next
		}
	default:
		squelch.nextPhase = next
	}
}

func (squelch *narrowFMSquelch) updateNoiseFloor() {
	value := math.Min(squelch.preFilter.capped, squelch.noiseFloor)
	squelch.noiseFloor = squelch.noiseFloor*0.97 + value*0.03 + 1e-6
	squelch.recalculateMovingAverageCap()
}

func (squelch *narrowFMSquelch) recalculateMovingAverageCap() {
	if squelch.usingManualThreshold {
		squelch.movingAverageCap = 1.5 * squelch.manualThreshold
		return
	}
	squelch.movingAverageCap = 1.5 * squelch.normalSignalRatio * squelch.noiseFloor
}

func (squelch *narrowFMSquelch) updateMovingAverage(average *movingAverage, sample float64) {
	average.full = average.full*0.99 + sample*0.01
	if average.capped >= squelch.movingAverageCap && sample >= squelch.movingAverageCap {
		average.capped = squelch.movingAverageCap
		return
	}
	average.capped = math.Min(squelch.movingAverageCap, average.capped*0.99+sample*0.01)
}

func (squelch *narrowFMSquelch) level() float64 {
	if squelch.usingManualThreshold {
		return squelch.manualThreshold
	}
	if squelch.isFlapping() {
		return squelch.flappySignalRatio * squelch.noiseFloor
	}
	return squelch.normalSignalRatio * squelch.noiseFloor
}

func (squelch *narrowFMSquelch) hasPreFilterSignal() bool {
	return squelch.preFilter.capped >= squelch.level()
}

func (squelch *narrowFMSquelch) hasPostFilterSignal() bool {
	return squelch.usingPostFilter && squelch.postFilter.capped >= squelch.buffer[squelch.bufferTail]
}

func (squelch *narrowFMSquelch) hasSignal() bool {
	if squelch.usingPostFilter {
		return squelch.hasPreFilterSignal() && squelch.hasPostFilterSignal()
	}
	return squelch.hasPreFilterSignal()
}

func (squelch *narrowFMSquelch) currentSNRDB() float64 {
	noise := squelch.noiseFloor
	if noise <= 1e-12 {
		return -200
	}
	signal := squelch.preFilter.full
	if signal <= 1e-12 {
		return -200
	}
	ratio := signal / noise
	if ratio <= 1e-12 {
		return -200
	}
	return 20 * math.Log10(ratio)
}

func (squelch *narrowFMSquelch) currentSignalDBFS() float64 {
	signal := squelch.preFilter.full
	if signal <= 1e-12 {
		return -200
	}
	return 20 * math.Log10(signal)
}

func (squelch *narrowFMSquelch) isFlapping() bool {
	return squelch.recentOpenCount >= squelch.flapOpensThreshold
}

type toneDetector struct {
	frequency   float64
	coefficient float64
	windowSize  int
	count       int
	q1          float64
	q2          float64
	power       float64
}

func newToneDetector(frequency, sampleRate float64, windowSize int) toneDetector {
	bin := int(0.5 + float64(windowSize)*frequency/sampleRate)
	omega := 2 * math.Pi * float64(bin) / float64(windowSize)
	return toneDetector{
		frequency:   frequency,
		coefficient: 2 * math.Cos(omega),
		windowSize:  windowSize,
	}
}

func (detector *toneDetector) process(sample float64) {
	q0 := detector.coefficient*detector.q1 - detector.q2 + sample
	detector.q2 = detector.q1
	detector.q1 = q0
	detector.count++
	if detector.count == detector.windowSize {
		detector.power = detector.q1*detector.q1 + detector.q2*detector.q2 - detector.q1*detector.q2*detector.coefficient
		detector.count = 0
	}
}

func (detector *toneDetector) reset() {
	detector.count = 0
	detector.q1 = 0
	detector.q2 = 0
}

type ctcssDetector struct {
	detectors     []toneDetector
	windowSize    int
	sampleCount   int
	enoughSamples bool
	hasTone       bool
	foundCount    int
	notFoundCount int
}

func newCTCSSDetector(targetHz, sampleRate, windowSamples float64) (*ctcssDetector, error) {
	if targetHz <= 0 || targetHz >= sampleRate/2 {
		return nil, fmt.Errorf("invalid CTCSS frequency %.3f Hz", targetHz)
	}
	windowSize := int(math.Round(windowSamples))
	if windowSize < 1 {
		return nil, fmt.Errorf("invalid CTCSS detection window")
	}

	detector := &ctcssDetector{windowSize: windowSize}
	detector.addTone(targetHz, sampleRate)
	for _, tone := range standardCTCSSTones {
		if math.Abs(targetHz-tone) >= 5 {
			detector.addTone(tone, sampleRate)
		}
	}
	return detector, nil
}

func (detector *ctcssDetector) addTone(frequency, sampleRate float64) {
	candidate := newToneDetector(frequency, sampleRate, detector.windowSize)
	for _, existing := range detector.detectors {
		if math.Abs(candidate.coefficient-existing.coefficient) < 1e-12 {
			return
		}
	}
	detector.detectors = append(detector.detectors, candidate)
}

func (detector *ctcssDetector) process(sample float64) {
	for index := range detector.detectors {
		detector.detectors[index].process(sample)
	}
	detector.sampleCount++
	if detector.sampleCount < detector.windowSize {
		return
	}

	detector.enoughSamples = true
	targetPower := detector.detectors[0].power
	maximumPower := targetPower
	totalPower := 0.0
	for _, candidate := range detector.detectors {
		totalPower += candidate.power
		if candidate.power > maximumPower {
			maximumPower = candidate.power
		}
	}
	averagePower := totalPower / float64(len(detector.detectors))
	detector.hasTone = targetPower >= maximumPower && targetPower > averagePower
	if detector.hasTone {
		detector.foundCount++
	} else {
		detector.notFoundCount++
	}
	for index := range detector.detectors {
		detector.detectors[index].reset()
	}
	detector.sampleCount = 0
}

func (detector *ctcssDetector) reset() {
	detector.sampleCount = 0
	detector.enoughSamples = false
	detector.hasTone = false
	for index := range detector.detectors {
		detector.detectors[index].reset()
	}
}

var standardCTCSSTones = []float64{
	67.0, 69.3, 71.9, 74.4, 77.0, 79.7, 82.5, 85.4, 88.5, 91.5,
	94.8, 97.4, 100.0, 103.5, 107.2, 110.9, 114.8, 118.8, 123.0,
	127.3, 131.8, 136.5, 141.3, 146.2, 150.0, 151.4, 156.7, 159.8,
	162.2, 165.5, 167.9, 171.3, 173.8, 177.3, 179.9, 183.5, 186.2,
	189.9, 192.8, 196.6, 199.5, 203.5, 206.5, 210.7, 218.1, 225.7,
	229.1, 233.6, 241.8, 250.3, 254.1,
}
