package sdrhelper

import (
	"clearlink/internal/config"
	"clearlink/internal/models"
	"context"
	"fmt"
	"math"
	"time"

	rtlsdr "github.com/jpoirier/gortlsdr"
)

// Biquad filter state (for cascaded low-pass)
type biquad struct {
	b0, b1, b2, a1, a2 float64
	x1, x2, y1, y2     float64
}

func (f *biquad) process(x float64) float64 {
	y := f.b0*x + f.b1*f.x1 + f.b2*f.x2 - f.a1*f.y1 - f.a2*f.y2
	f.x2, f.x1 = f.x1, x
	f.y2, f.y1 = f.y1, y
	return y
}

func polarDiscriminant(ar, aj, br, bj int) int {
	cr := ar*br - aj*-bj
	cj := aj*br + ar*-bj
	angle := math.Atan2(float64(cj), float64(cr))
	return int(angle / math.Pi * (1 << 14))
}

// Create 2nd-order Butterworth low-pass biquad
func newLowpassBiquad(fc, fs float64, qFactor float64) *biquad {
	w0 := 2 * math.Pi * fc / fs
	cosw0 := math.Cos(w0)
	sinw0 := math.Sin(w0)
	alpha := sinw0 / (2 * qFactor)

	b0 := (1 - cosw0) / 2
	b1 := 1 - cosw0
	b2 := (1 - cosw0) / 2
	a0 := 1 + alpha
	a1 := -2 * cosw0
	a2 := 1 - alpha

	return &biquad{
		b0: b0 / a0,
		b1: b1 / a0,
		b2: b2 / a0,
		a1: a1 / a0,
		a2: a2 / a0,
	}
}

// SDRData holds the output from each SDR loop iteration
type SDRData struct {
	RSSI       float64 // Average signal strength in dB for this chunk
	SampleRate int     // Sample rate for AudioChunk
	AudioChunk []int16 // 16-bit PCM mono audio chunk
}

// SdrLoop runs the SDR receiver and sends data over the channel
func SdrLoop(ctx context.Context, dataChan chan<- SDRData) {
	defer close(dataChan)

	frequency := config.GetConfigValue("Frequency", models.ApplicationTypeListen).(int)
	sampleRate := config.GetConfigValue("SampleRate", models.ApplicationTypeListen).(int)
	audioRate := config.GetConfigValue("AudioRate", models.ApplicationTypeListen).(int)
	audioChunkMs := config.GetConfigValue("AudioChunkMs", models.ApplicationTypeListen).(int)
	bufferSize := config.GetConfigValue("BufferSize", models.ApplicationTypeListen).(int)
	audioGain := config.GetConfigValue("AudioGain", models.ApplicationTypeListen).(int)
	fmDeviation := float64(config.GetConfigValue("FMDeviation", models.ApplicationTypeListen).(int))
	audioCutoffHz := float64(config.GetConfigValue("AudioCutoffHz", models.ApplicationTypeListen).(int))
	deemphasisTauUs := config.GetConfigValue("DeemphasisTauUs", models.ApplicationTypeListen).(int)
	tunerBandwidth := config.GetConfigValue("TunerBandwidth", models.ApplicationTypeListen).(int)
	tunerGain := config.GetConfigValue("TunerGain", models.ApplicationTypeListen).(int)
	aaFilterTaps := config.GetConfigValue("AAFilterTaps", models.ApplicationTypeListen).(int)

	if sampleRate%audioRate != 0 {
		config.UpdateConfigValue(models.ConfigEntry{
			Key:  "AudioRate",
			Type: models.ApplicationTypeListen,
			Var: models.EntryVar{
				Type: models.EntryTypeInt,
				Data: 48000,
			},
			Default: models.EntryVar{
				Type: models.EntryTypeInt,
				Data: 48000,
			},
		})
		config.UpdateConfigValue(models.ConfigEntry{
			Key:  "SampleRate",
			Type: models.ApplicationTypeListen,
			Var: models.EntryVar{
				Type: models.EntryTypeInt,
				Data: 960000,
			},
			Default: models.EntryVar{
				Type: models.EntryTypeInt,
				Data: 960000,
			},
		})
		print(fmt.Sprintf(
			"invalid SDR sample rate: %d is not evenly divisible by audio rate %d; "+
				"reset to 960000 : 48000",
			sampleRate,
			audioRate,
		))
	}

	decimation := sampleRate / audioRate

	if decimation < 1 {
		panic(fmt.Sprintf(
			"invalid decimation ratio: %d",
			decimation,
		))
	}

	chunkSamples := audioRate * audioChunkMs / 1000
	deemphasisTau := float64(deemphasisTauUs) * 1e-6

	dev, err := rtlsdr.Open(0)
	if err != nil {
		panic(err)
	}
	defer dev.Close()

	fmt.Println("RTL-SDR opened")

	dev.SetCenterFreq(frequency)
	dev.SetSampleRate(sampleRate)
	dev.SetTunerGainMode(true)
	dev.SetTunerGain(tunerGain)
	dev.SetTunerBw(tunerBandwidth)
	dev.ResetBuffer()

	fmt.Printf("Monitoring %.3f MHz\n", float64(frequency)/1e6)

	demodScale := float64(sampleRate) / (2 * math.Pi * fmDeviation)
	deemphAlpha := math.Exp(-1.0 / (float64(audioRate) * deemphasisTau))

	lpFilter1 := newLowpassBiquad(audioCutoffHz, float64(audioRate), 0.5412)
	lpFilter2 := newLowpassBiquad(audioCutoffHz, float64(audioRate), 1.3065)

	buf := make([]uint8, bufferSize)

	var phaseReal float64
	var phaseImag float64
	var prev complex64
	var prevI int
	var prevQ int
	var hasPrevIQ bool
	var deemph float64

	aaBuffer := make([]float64, aaFilterTaps)
	aaIndex := 0
	aaSum := 0.0

	sampleCount := 0
	audioAccumulator := 0.0
	var powerSum float64
	var powerSamples int
	audioChunk := make([]int16, 0, chunkSamples)
	chunkPowerSum := 0.0
	chunkPowerSamples := 0

	bufferIQSamples := bufferSize / 2

	bufferDuration := time.Duration(float64(bufferIQSamples) / float64(sampleRate) * float64(time.Second))

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := dev.ReadSync(buf, len(buf))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			panic(err)
		}

		powerSum = 0.0
		powerSamples = 0
		if true {//RaspberryPiGeneration() != "pi3" { // Use polar discriminant FM demodulation on Pi 4 and newer.
			for i := 0; i+1 < n; i += 2 {
				iqI := (float32(buf[i]) - 127.5) / 127.5
				iqQ := (float32(buf[i+1]) - 127.5) / 127.5

				currI := int(buf[i]) - 127
				currQ := int(buf[i+1]) - 127

				power := float64(iqI*iqI + iqQ*iqQ)
				powerSum += power
				powerSamples++

				if !hasPrevIQ {
					prevI = currI
					prevQ = currQ
					hasPrevIQ = true
					continue
				}

				pcm := polarDiscriminant(currI, currQ, prevI, prevQ)
				audioVal := float64(pcm) * math.Pi / float64(1<<14)

				prevI = currI
				prevQ = currQ

				aaSum -= aaBuffer[aaIndex]
				aaBuffer[aaIndex] = audioVal
				aaSum += audioVal
				aaIndex = (aaIndex + 1) % aaFilterTaps

				audioFiltered := aaSum / float64(aaFilterTaps)

				audioAccumulator += audioFiltered
				sampleCount++

				if sampleCount >= decimation {
					audioSample := audioAccumulator / float64(sampleCount)
					audioSample *= demodScale

					audioAccumulator = 0
					sampleCount = 0

					deemph = deemph*deemphAlpha + audioSample*(1-deemphAlpha)
					audioSample = deemph

					audioSample = lpFilter1.process(audioSample)
					audioSample = lpFilter2.process(audioSample)

					sample := int(audioSample * float64(audioGain))
					if sample > 32767 {
						sample = 32767
					} else if sample < -32768 {
						sample = -32768
					}

					audioChunk = append(audioChunk, int16(sample))
					chunkPowerSum += powerSum
					chunkPowerSamples += powerSamples

					if len(audioChunk) >= chunkSamples {
						rssi := 0.0
						if chunkPowerSamples > 0 {
							rssi = 10 * math.Log10(chunkPowerSum/float64(chunkPowerSamples))
						}

						chunk := make([]int16, len(audioChunk))
						copy(chunk, audioChunk)

						select {
						case dataChan <- SDRData{
							RSSI:       rssi,
							SampleRate: audioRate,
							AudioChunk: chunk,
						}:
						default:
							// Keep latency bounded by dropping chunks when the consumer falls behind.
						}

						audioChunk = audioChunk[:0]
						chunkPowerSum = 0.0
						chunkPowerSamples = 0
					}
				}
			}
		} else { // Use imag(v) for FM demodulation on Pi 3 ( Uses less CPU but sounds worse )
			processStart := time.Now()

			for i := 0; i+1 < n; i += 2 {
				// Convert unsigned RTL-SDR IQ to normalized [-1, 1].
				iqI := (float32(buf[i]) - 127.5) / 127.5
				iqQ := (float32(buf[i+1]) - 127.5) / 127.5

				curr := complex(iqI, iqQ)

				// ---------------------------------------------------------
				// RF power measurement
				// ---------------------------------------------------------

				power := float64(iqI*iqI + iqQ*iqQ)

				chunkPowerSum += power
				chunkPowerSamples++

				// FM discriminator.
				// For narrowband FM, the phase difference between adjacent IQ
				// samples is small, so imag(v) is an excellent approximation of
				// the instantaneous phase difference.
				//
				// This avoids an atan2() call for every IQ sample.
				if sampleCount > 0 {
					v := complex128(curr) * complex128(complex(
						real(prev),
						-imag(prev),
					))

					phaseReal += real(v)
					phaseImag += imag(v)
				}

				prev = curr
				sampleCount++

				if sampleCount >= decimation {
					// Average the phase-difference vectors.
					phaseReal /= float64(sampleCount)
					phaseImag /= float64(sampleCount)

					// One atan2() per audio sample.
					audioVal := math.Atan2(phaseImag, phaseReal)

					// Convert phase change to normalized FM deviation.
					audioVal *= demodScale

					phaseReal = 0
					phaseImag = 0
					sampleCount = 0

					// De-emphasis
					deemph = deemph*deemphAlpha +
						audioVal*(1.0-deemphAlpha)

					audioVal = deemph

					// Audio filtering
					audioVal = lpFilter1.process(audioVal)
					audioVal = lpFilter2.process(audioVal)

					// Gain
					sample := int(audioVal * float64(audioGain))

					if sample > 32767 {
						sample = 32767
					} else if sample < -32768 {
						sample = -32768
					}

					audioChunk = append(audioChunk, int16(sample))

					// Emit complete audio chunk
					if len(audioChunk) >= chunkSamples {
						rssi := -100.0

						if chunkPowerSamples > 0 {
							meanPower := chunkPowerSum /
								float64(chunkPowerSamples)

							if meanPower > 0 {
								rssi = 10 * math.Log10(meanPower)
							}
						}

						chunk := make([]int16, len(audioChunk))
						copy(chunk, audioChunk)

						select {
						case dataChan <- SDRData{
							RSSI:       rssi,
							SampleRate: audioRate,
							AudioChunk: chunk,
						}:
						default:
						}

						audioChunk = audioChunk[:0]
						chunkPowerSum = 0
						chunkPowerSamples = 0
					}
				}
			}

			processDuration := time.Since(processStart)

			if processDuration > bufferDuration {
				fmt.Printf("[SDR WARNING] Processing is falling behind the SDR stream!\n")
			}
		}
	}
}