package main

import (
	"clearlink/internal/network"
	"clearlink/internal/platform"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/warthog618/go-gpiocdev"
)

const (
	interfaceAudioQueueChunks = 4
	interfacePTTHoldDuration  = 250 * time.Millisecond
	interfaceProcessStopGrace = 500 * time.Millisecond
)

type SoundcardOutput struct {
	mu           sync.Mutex
	device       string
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	sampleRate   uint32
	ptt          *GPIOOutput
	queue        chan *network.ToAnyAudioChunkPacket
	quitChan     chan struct{}
	closeOnce    sync.Once
	wg           sync.WaitGroup
	transmitting bool
	lastAudioAt  time.Time
}

func NewSoundcardOutput(device string, pttPin int, pttActiveHigh bool) (*SoundcardOutput, error) {
	ptt, err := NewGPIOOutput(pttPin, pttActiveHigh)
	if err != nil {
		return nil, err
	}

	output := &SoundcardOutput{
		device:   device,
		ptt:      ptt,
		queue:    make(chan *network.ToAnyAudioChunkPacket, interfaceAudioQueueChunks),
		quitChan: make(chan struct{}),
	}
	output.wg.Add(1)
	go output.playLoop()
	return output, nil
}

func (output *SoundcardOutput) SendAudio(chunk *network.ToAnyAudioChunkPacket) {
	if chunk == nil {
		return
	}
	copied := &network.ToAnyAudioChunkPacket{
		ChunkNumber: chunk.ChunkNumber,
		SampleRate:  chunk.SampleRate,
		Samples:     append([]int16(nil), chunk.Samples...),
	}

	select {
	case output.queue <- copied:
	case <-output.quitChan:
	default:
		select {
		case <-output.queue:
		default:
		}
		select {
		case output.queue <- copied:
		case <-output.quitChan:
		default:
		}
	}
}

func (output *SoundcardOutput) NotifyTransmitStop() {
	output.setPTT(false)
}

func (output *SoundcardOutput) playLoop() {
	defer output.wg.Done()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-output.quitChan:
			return
		case <-ticker.C:
			output.mu.Lock()
			idleTooLong := output.transmitting && !output.lastAudioAt.IsZero() && time.Since(output.lastAudioAt) > interfacePTTHoldDuration
			output.mu.Unlock()
			if idleTooLong {
				output.setPTT(false)
			}
		case chunk := <-output.queue:
			if len(chunk.Samples) == 0 {
				output.NotifyTransmitStop()
				continue
			}
			if err := output.writeChunk(chunk); err != nil {
				fmt.Printf("Interface soundcard playback error: %v\n", err)
			}
		}
	}
}

func (output *SoundcardOutput) writeChunk(chunk *network.ToAnyAudioChunkPacket) error {
	output.mu.Lock()
	if err := output.ensureProcessLocked(chunk.SampleRate); err != nil {
		output.mu.Unlock()
		return err
	}
	stdin := output.stdin
	output.lastAudioAt = time.Now()
	output.mu.Unlock()

	output.setPTT(true)
	pcm := make([]byte, len(chunk.Samples)*2)
	for index, sample := range chunk.Samples {
		binary.LittleEndian.PutUint16(pcm[index*2:], uint16(sample))
	}
	if _, err := stdin.Write(pcm); err != nil {
		output.mu.Lock()
		output.stopProcessLocked()
		output.mu.Unlock()
		return fmt.Errorf("write to aplay: %w", err)
	}
	return nil
}

func (output *SoundcardOutput) ensureProcessLocked(sampleRate uint32) error {
	if sampleRate == 0 {
		sampleRate = 48000
	}
	if output.cmd != nil && output.stdin != nil && output.sampleRate == sampleRate {
		return nil
	}

	output.stopProcessLocked()
	args := []string{"-q", "-D", output.device, "-f", "S16_LE", "-c", "1", "-r", strconv.Itoa(int(sampleRate)), "-B", "40000", "-F", "10000", "-t", "raw"}
	cmd := exec.Command("aplay", args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open aplay stdin: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start aplay: %w", err)
	}

	output.cmd = cmd
	output.stdin = stdin
	output.sampleRate = sampleRate
	return nil
}

func (output *SoundcardOutput) stopProcessLocked() {
	if output.stdin != nil {
		_ = output.stdin.Close()
		output.stdin = nil
	}
	if output.cmd == nil {
		return
	}

	cmd := output.cmd
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(interfaceProcessStopGrace):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
	}
	output.cmd = nil
	output.sampleRate = 0
}

func (output *SoundcardOutput) setPTT(active bool) {
	output.mu.Lock()
	already := output.transmitting == active
	output.transmitting = active
	output.mu.Unlock()
	if already {
		return
	}
	if err := output.ptt.Set(active); err != nil {
		fmt.Printf("Failed to set interface PTT=%t: %v\n", active, err)
	}
}

func (output *SoundcardOutput) Close() error {
	var closeErr error
	output.closeOnce.Do(func() {
		close(output.quitChan)
		output.wg.Wait()
		output.setPTT(false)
		output.mu.Lock()
		output.stopProcessLocked()
		output.mu.Unlock()
		closeErr = output.ptt.Close()
	})
	return closeErr
}

type SquelchInput struct {
	enabled bool
	chip    *gpiocdev.Chip
	line    *gpiocdev.Line
}

func NewSquelchInput(pin int, activeHigh bool) (*SquelchInput, error) {
	return newSquelchInput(pin, activeHigh, platform.IsRaspberryPi())
}

func newSquelchInput(pin int, activeHigh bool, enabled bool) (*SquelchInput, error) {
	if pin < 0 {
		return nil, fmt.Errorf("invalid GPIO pin: %d", pin)
	}
	if !enabled {
		fmt.Println("GPIO squelch disabled: host is not a Raspberry Pi; interface input is always live")
		return &SquelchInput{}, nil
	}
	chip, err := gpiocdev.NewChip("gpiochip0")
	if err != nil {
		return nil, fmt.Errorf("open gpiochip0: %w", err)
	}

	var line *gpiocdev.Line
	if activeHigh {
		line, err = chip.RequestLine(pin, gpiocdev.AsInput)
	} else {
		line, err = chip.RequestLine(pin, gpiocdev.AsInput, gpiocdev.AsActiveLow)
	}
	if err != nil {
		_ = chip.Close()
		return nil, fmt.Errorf("request GPIO %d as input: %w", pin, err)
	}
	return &SquelchInput{enabled: true, chip: chip, line: line}, nil
}

func (input *SquelchInput) IsOpen() (bool, error) {
	if !input.enabled {
		return true, nil
	}
	if input.line == nil {
		return false, errors.New("squelch GPIO is not initialized")
	}
	value, err := input.line.Value()
	if err != nil {
		return false, err
	}
	return value == 1, nil
}

func (input *SquelchInput) Close() error {
	if !input.enabled {
		return nil
	}
	if input.line != nil {
		if err := input.line.Close(); err != nil {
			return err
		}
		input.line = nil
	}
	if input.chip != nil {
		if err := input.chip.Close(); err != nil {
			return err
		}
		input.chip = nil
	}
	return nil
}

type GPIOOutput struct {
	enabled bool
	chip    *gpiocdev.Chip
	line    *gpiocdev.Line
}

func NewGPIOOutput(pin int, activeHigh bool) (*GPIOOutput, error) {
	return newGPIOOutput(pin, activeHigh, platform.IsRaspberryPi())
}

func newGPIOOutput(pin int, activeHigh bool, enabled bool) (*GPIOOutput, error) {
	if pin < 0 {
		return nil, fmt.Errorf("invalid GPIO pin: %d", pin)
	}
	if !enabled {
		fmt.Println("GPIO PTT disabled: host is not a Raspberry Pi")
		return &GPIOOutput{}, nil
	}
	chip, err := gpiocdev.NewChip("gpiochip0")
	if err != nil {
		return nil, fmt.Errorf("open gpiochip0: %w", err)
	}

	var line *gpiocdev.Line
	if activeHigh {
		line, err = chip.RequestLine(pin, gpiocdev.AsOutput(0))
	} else {
		line, err = chip.RequestLine(pin, gpiocdev.AsOutput(0), gpiocdev.AsActiveLow)
	}
	if err != nil {
		_ = chip.Close()
		return nil, fmt.Errorf("request GPIO %d as output: %w", pin, err)
	}
	output := &GPIOOutput{enabled: true, chip: chip, line: line}
	if err := output.Set(false); err != nil {
		_ = output.Close()
		return nil, err
	}
	return output, nil
}

func (output *GPIOOutput) Set(active bool) error {
	if !output.enabled {
		return nil
	}
	if output.line == nil {
		return errors.New("PTT GPIO is not initialized")
	}
	value := 0
	if active {
		value = 1
	}
	if err := output.line.SetValue(value); err != nil {
		return err
	}
	return nil
}

func (output *GPIOOutput) Close() error {
	if !output.enabled {
		return nil
	}
	if output.line != nil {
		if err := output.Set(false); err != nil {
			return err
		}
		if err := output.line.Close(); err != nil {
			return err
		}
		output.line = nil
	}
	if output.chip != nil {
		if err := output.chip.Close(); err != nil {
			return err
		}
		output.chip = nil
	}
	return nil
}
