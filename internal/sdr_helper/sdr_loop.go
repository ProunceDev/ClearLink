//go:build cgo

package sdrhelper

import (
	"clearlink/internal/config"
	"clearlink/internal/models"
	"context"
	"fmt"
	"math"

	rtlsdr "github.com/jpoirier/gortlsdr"
)

// SdrLoop runs the configured narrow-FM receiver and emits PCM chunks.
func SdrLoop(ctx context.Context, dataChan chan<- SDRData) {
	defer close(dataChan)
	if err := sdrLoopNarrowFM(ctx, dataChan); err != nil {
		fmt.Printf("narrow-FM receiver stopped: %v\n", err)
	}
}

func sdrLoopNarrowFM(ctx context.Context, dataChan chan<- SDRData) error {
	if config.Config == nil {
		return fmt.Errorf("listen configuration has not been loaded")
	}

	settings, err := narrowFMSettingsFromEntries(config.Config.Entries)
	if err != nil {
		return err
	}
	if err := ValidateListenConfig(*config.Config); err != nil {
		return err
	}

	processor, err := newNarrowFMProcessor(settings)
	if err != nil {
		return fmt.Errorf("invalid narrow-FM configuration: %w", err)
	}

	chunkSamples, err := audioChunkSampleCount(settings.OutputSampleRate, listenIntConfig("AudioChunkMs"))
	if err != nil {
		return err
	}
	bufferSize := listenIntConfig("BufferSize")
	if bufferSize%2 != 0 {
		bufferSize--
	}

	deviceIndex := listenIntConfig("DeviceIndex")
	dev, err := rtlsdr.Open(deviceIndex)
	if err != nil {
		return fmt.Errorf("open RTL-SDR device %d: %w", deviceIndex, err)
	}
	defer dev.Close()

	if err := configureRTLSDR(dev, settings); err != nil {
		return err
	}

	audioChunk := make([]int16, 0, chunkSamples)
	buf := make([]uint8, bufferSize)
	chunkSNRSum := 0.0
	chunkSNRSamples := 0
	chunkHasAudio := false
	chunkSquelchOpen := false
	audioGain := float64(listenIntConfig("AudioGain"))

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		n, err := dev.ReadSync(buf, len(buf))
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read RTL-SDR samples: %w", err)
		}

		for index := 0; index+1 < n; index += 2 {
			rawI := (float64(buf[index]) - 127.5) / 127.5
			rawQ := (float64(buf[index+1]) - 127.5) / 127.5

			audioSample, emitted, squelchOpen := processor.processIQ(rawI, rawQ)
			if !emitted {
				continue
			}
			chunkSNRSum += processor.currentSNRDB()
			chunkSNRSamples++

			sample := int(math.Round(audioSample * audioGain))
			if sample > math.MaxInt16 {
				sample = math.MaxInt16
			} else if sample < math.MinInt16 {
				sample = math.MinInt16
			}
			audioChunk = append(audioChunk, int16(sample))
			chunkHasAudio = chunkHasAudio || squelchOpen
			chunkSquelchOpen = squelchOpen

			if len(audioChunk) < chunkSamples {
				continue
			}

			rssi := processor.currentRSSIDBFS()

			snr := -200.0
			if chunkSNRSamples > 0 {
				snr = chunkSNRSum / float64(chunkSNRSamples)
			}

			chunk := make([]int16, len(audioChunk))
			copy(chunk, audioChunk)
			select {
			case dataChan <- SDRData{
				RSSI:        rssi,
				SNR:         snr,
				SampleRate:  settings.OutputSampleRate,
				AudioChunk:  chunk,
				SquelchOpen: chunkSquelchOpen,
				HasAudio:    chunkHasAudio,
			}:
			default:
			}

			audioChunk = audioChunk[:0]
			chunkSNRSum = 0
			chunkSNRSamples = 0
			chunkHasAudio = false
		}
	}
}

func listenIntConfig(key string) int {
	value, ok := config.GetConfigValue(key, models.ApplicationTypeListen).(int)
	if !ok {
		panic(fmt.Sprintf("listen config %s is not an integer", key))
	}
	return value
}

func nearestTunerGain(gains []int, requested int) int {
	if len(gains) == 0 {
		return requested
	}

	nearest := gains[0]
	for _, gain := range gains[1:] {
		if math.Abs(float64(gain-requested)) < math.Abs(float64(nearest-requested)) {
			nearest = gain
		}
	}
	return nearest
}

func configureRTLSDR(dev *rtlsdr.Context, settings narrowFMSettings) error {
	directSampling := rtlsdr.SamplingMode(listenIntConfig("DirectSampling"))
	switch directSampling {
	case rtlsdr.SamplingNone, rtlsdr.SamplingIADC, rtlsdr.SamplingQADC:
	default:
		return fmt.Errorf("invalid direct sampling mode %d", directSampling)
	}
	if err := dev.SetDirectSampling(directSampling); err != nil {
		return fmt.Errorf("set direct sampling: %w", err)
	}
	centerFrequency := listenIntConfig("CenterFrequency")
	if centerFrequency <= 0 {
		centerFrequency = listenIntConfig("Frequency")
	}
	if err := dev.SetCenterFreq(centerFrequency); err != nil {
		return fmt.Errorf("set center frequency: %w", err)
	}
	if err := dev.SetSampleRate(settings.InputSampleRate); err != nil {
		return fmt.Errorf("set sample rate: %w", err)
	}
	if err := dev.SetTunerGainMode(true); err != nil {
		fmt.Printf("warning: SetTunerGainMode failed: %v\n", err)
	}
	requestedGain := listenIntConfig("TunerGain")
	if gains, err := dev.GetTunerGains(); err == nil && len(gains) > 0 {
		gain := nearestTunerGain(gains, requestedGain)
		if gain != requestedGain {
			fmt.Printf("RTL-SDR requested tuner gain %d (x10 dB) is unavailable; using %d (x10 dB)\n", requestedGain, gain)
		}
		if err := dev.SetTunerGain(gain); err != nil {
			fmt.Printf("warning: SetTunerGain failed: %v\n", err)
		}
		fmt.Printf("RTL-SDR supported gains (x10 dB): %v\n", gains)
	} else if err := dev.SetTunerGain(requestedGain); err != nil {
		fmt.Printf("warning: SetTunerGain failed: %v\n", err)
	}
	if err := dev.SetAgcMode(false); err != nil {
		fmt.Printf("warning: SetAgcMode failed: %v\n", err)
	}

	tunerBandwidth := listenIntConfig("TunerBandwidth")
	targetOffset := math.Abs(float64(settings.TargetFrequencyHz - centerFrequency))
	minimumTunerBandwidth := int(math.Ceil(2*targetOffset + settings.ChannelBandwidthHz))
	if tunerBandwidth > 0 && tunerBandwidth < minimumTunerBandwidth {
		fmt.Printf("warning: tuner bandwidth %d Hz cannot cover a %.0f Hz offset and %.0f Hz channel; using automatic bandwidth\n", tunerBandwidth, targetOffset, settings.ChannelBandwidthHz)
		tunerBandwidth = 0
	}
	if targetOffset >= 0.45*float64(settings.InputSampleRate) {
		fmt.Printf("warning: target is %.0f Hz from center; choose a center within %.0f Hz for reliable reception\n", targetOffset, 0.45*float64(settings.InputSampleRate))
	}
	if err := dev.SetTunerBw(tunerBandwidth); err != nil {
		fmt.Printf("warning: SetTunerBw failed: %v\n", err)
	}
	if err := dev.ResetBuffer(); err != nil {
		return fmt.Errorf("reset RTL-SDR buffer: %w", err)
	}

	if centerFrequency != listenIntConfig("Frequency") {
		fmt.Printf("Monitoring %.3f MHz at center %.3f MHz\n", float64(listenIntConfig("Frequency"))/1e6, float64(centerFrequency)/1e6)
	} else {
		fmt.Printf("Monitoring %.3f MHz\n", float64(listenIntConfig("Frequency"))/1e6)
	}
	fmt.Printf("NFM input: %d Hz, audio: %d Hz, channel bandwidth: %.0f Hz\n", settings.InputSampleRate, settings.OutputSampleRate, settings.ChannelBandwidthHz)
	return nil
}
