package main

import (
	"clearlink/internal/network"
	"clearlink/internal/platform"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/warthog618/go-gpiocdev"
)

const (
	radioQueueChunks      = 4
	radioPTTCheckInterval = 10 * time.Millisecond
	radioProcessStopGrace = 500 * time.Millisecond
)

type RadioPlayer struct {
	mu           sync.Mutex
	cmd          *exec.Cmd
	stdin        *os.File
	sampleRate   uint32
	ptt          *GPIOPTT
	queue        chan *network.ToAnyAudioChunkPacket
	quitChan     chan struct{}
	wg           sync.WaitGroup
	transmitting bool
	lastAudioAt  time.Time
	pttHangTime  time.Duration
}

func NewRadioPlayer(pttPin int, pttHangTime time.Duration) (*RadioPlayer, error) {
	if pttHangTime < 0 {
		return nil, fmt.Errorf("PTT hang time must be non-negative")
	}
	ptt, err := NewGPIOPTT(pttPin)
	if err != nil {
		return nil, err
	}

	rp := &RadioPlayer{
		ptt:         ptt,
		queue:       make(chan *network.ToAnyAudioChunkPacket, radioQueueChunks),
		quitChan:    make(chan struct{}),
		pttHangTime: pttHangTime,
	}

	rp.wg.Add(1)
	go rp.playLoop()

	return rp, nil
}

func (rp *RadioPlayer) SendAudio(chunk *network.ToAnyAudioChunkPacket) {
	if chunk == nil {
		return
	}

	copied := &network.ToAnyAudioChunkPacket{
		ChunkNumber: chunk.ChunkNumber,
		SampleRate:  chunk.SampleRate,
		Samples:     append([]int16(nil), chunk.Samples...),
	}

	select {
	case rp.queue <- copied:
	case <-rp.quitChan:
	default:
		select {
		case <-rp.queue:
		default:
		}
		select {
		case rp.queue <- copied:
		case <-rp.quitChan:
		default:
		}
	}
}

func (rp *RadioPlayer) NotifyTransmitStop() {
	rp.setPTT(false)
}

func (rp *RadioPlayer) playLoop() {
	defer rp.wg.Done()
	ticker := time.NewTicker(radioPTTCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rp.quitChan:
			return
		case <-ticker.C:
			rp.mu.Lock()
			idleTooLong := rp.transmitting && pttHangTimeExpired(rp.lastAudioAt, time.Now(), rp.pttHangTime)
			rp.mu.Unlock()
			if idleTooLong {
				rp.setPTT(false)
			}
		case chunk, open := <-rp.queue:
			if !open {
				return
			}
			if len(chunk.Samples) == 0 {
				rp.startPTTHang()
				continue
			}
			if err := rp.writeChunk(chunk); err != nil {
				fmt.Printf("Radio playback error: %v\n", err)
			}
		}
	}
}

func pttHangTimeExpired(lastAudioAt, now time.Time, hangTime time.Duration) bool {
	return !lastAudioAt.IsZero() && !now.Before(lastAudioAt.Add(hangTime))
}

func (rp *RadioPlayer) startPTTHang() {
	rp.mu.Lock()
	transmitting := rp.transmitting
	hangTime := rp.pttHangTime
	if transmitting && hangTime > 0 {
		rp.lastAudioAt = time.Now()
	}
	rp.mu.Unlock()

	if transmitting && hangTime == 0 {
		rp.setPTT(false)
	}
}

func (rp *RadioPlayer) writeChunk(chunk *network.ToAnyAudioChunkPacket) error {
	if len(chunk.Samples) == 0 {
		return nil
	}

	rp.mu.Lock()
	if err := rp.ensureProcessLocked(chunk.SampleRate); err != nil {
		rp.mu.Unlock()
		return err
	}
	stdin := rp.stdin
	rp.lastAudioAt = time.Now()
	rp.mu.Unlock()

	rp.setPTT(true)

	pcm := make([]byte, len(chunk.Samples)*2)
	for i, sample := range chunk.Samples {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(sample))
	}

	if _, err := stdin.Write(pcm); err != nil {
		rp.mu.Lock()
		rp.stopProcessLocked()
		rp.mu.Unlock()
		return fmt.Errorf("failed to write to aplay: %w", err)
	}

	return nil
}

func (rp *RadioPlayer) ensureProcessLocked(sampleRate uint32) error {
	if sampleRate == 0 {
		sampleRate = 48000
	}
	if rp.cmd != nil && rp.stdin != nil && rp.sampleRate == sampleRate {
		return nil
	}

	rp.stopProcessLocked()

	cmd := exec.Command(
		"aplay",
		"-q",
		"-f", "S16_LE",
		"-c", "1",
		"-r", strconv.Itoa(int(sampleRate)),
		"-B", "40000", // 40ms target buffer
		"-F", "10000", // 10ms period
		"-t", "raw",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to open aplay stdin: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start aplay: %w", err)
	}

	stdinFile, ok := stdin.(*os.File)
	if !ok {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return fmt.Errorf("unexpected aplay stdin type")
	}

	rp.cmd = cmd
	rp.stdin = stdinFile
	rp.sampleRate = sampleRate
	return nil
}

func (rp *RadioPlayer) stopProcessLocked() {
	if rp.stdin != nil {
		_ = rp.stdin.Close()
		rp.stdin = nil
	}
	if rp.cmd == nil {
		return
	}

	done := make(chan struct{})
	cmd := rp.cmd
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(radioProcessStopGrace):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
	}

	rp.cmd = nil
	rp.sampleRate = 0
}

func (rp *RadioPlayer) setPTT(active bool) {
	rp.mu.Lock()
	already := rp.transmitting == active
	rp.transmitting = active
	rp.mu.Unlock()
	if already {
		return
	}
	if err := rp.ptt.Set(active); err != nil {
		fmt.Printf("Failed to set PTT=%t: %v\n", active, err)
	}
}

func (rp *RadioPlayer) Close() error {
	close(rp.quitChan)
	rp.wg.Wait()
	close(rp.queue)

	rp.setPTT(false)

	rp.mu.Lock()
	rp.stopProcessLocked()
	rp.mu.Unlock()

	if rp.ptt != nil {
		return rp.ptt.Close()
	}
	return nil
}

type GPIOPTT struct {
	pin     int
	enabled bool
	chip    *gpiocdev.Chip
	line    *gpiocdev.Line
}

func NewGPIOPTT(pin int) (*GPIOPTT, error) {
	return newGPIOPTT(pin, platform.IsRaspberryPi())
}

func newGPIOPTT(pin int, enabled bool) (*GPIOPTT, error) {
	if pin < 0 {
		return nil, fmt.Errorf("invalid GPIO pin: %d", pin)
	}
	if !enabled {
		fmt.Println("GPIO PTT disabled: host is not a Raspberry Pi")
		return &GPIOPTT{pin: pin}, nil
	}

	chip, err := gpiocdev.NewChip("gpiochip0")
	if err != nil {
		return nil, fmt.Errorf("failed to open gpiochip0: %w", err)
	}

	line, err := chip.RequestLine(pin, gpiocdev.AsOutput(0))
	if err != nil {
		_ = chip.Close()
		return nil, fmt.Errorf("failed to request GPIO %d as output: %w", pin, err)
	}

	ptt := &GPIOPTT{
		pin:     pin,
		enabled: true,
		chip:    chip,
		line:    line,
	}

	if err := ptt.Set(false); err != nil {
		_ = line.Close()
		_ = chip.Close()
		return nil, err
	}

	return ptt, nil
}

func (g *GPIOPTT) Set(active bool) error {
	if !g.enabled {
		return nil
	}
	if g.line == nil {
		return fmt.Errorf("GPIO line %d is not initialized", g.pin)
	}

	value := 0
	if active {
		value = 1
	}

	if err := g.line.SetValue(value); err != nil {
		return fmt.Errorf("failed to set GPIO %d value to %d: %w", g.pin, value, err)
	}
	return nil
}

func (g *GPIOPTT) Close() error {
	if !g.enabled {
		return nil
	}
	if err := g.Set(false); err != nil {
		return err
	}

	if g.line != nil {
		if err := g.line.Close(); err != nil {
			return err
		}
		g.line = nil
	}

	if g.chip != nil {
		if err := g.chip.Close(); err != nil {
			return err
		}
		g.chip = nil
	}

	return nil
}
