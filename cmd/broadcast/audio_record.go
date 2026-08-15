package main

import (
	"clearlink/internal/network"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	audioOutputDir   = "recordings"
	maxPendingChunks = 8
	routedStreamID   = uint16(1)
	maxChunkGapAge   = 120 * time.Millisecond
	maxConcealChunks = 4
	recordQueueSize  = 32
)

type nodeAudioStream struct {
	file          *os.File
	sampleRate    uint32
	expectedChunk uint32
	pending       map[uint32][]int16
	bytesWritten  uint32
	chunkSize     int
	lastProgress  time.Time
}

var (
	audioStreams   = make(map[uint16]*nodeAudioStream)
	audioStreamsMu sync.Mutex

	recordQueue   chan *network.ToAnyAudioChunkPacket
	recorderOnce  sync.Once
	recorderStop  chan struct{}
	recorderWG    sync.WaitGroup
	recorderReady bool
)

func ensureAudioOutputDir() error {
	return os.MkdirAll(audioOutputDir, 0o755)
}

func startAudioRecorder() {
	recorderOnce.Do(func() {
		recordQueue = make(chan *network.ToAnyAudioChunkPacket, recordQueueSize)
		recorderStop = make(chan struct{})
		recorderReady = true

		recorderWG.Add(1)
		go func() {
			defer recorderWG.Done()
			for {
				select {
				case <-recorderStop:
					return
				case packet, ok := <-recordQueue:
					if !ok {
						return
					}
					if err := handleAudioChunk(packet); err != nil {
						fmt.Printf("Failed to handle routed audio packet for recording: %v\n", err)
					}
				}
			}
		}()
	})
}

func queueAudioChunkForRecording(packet *network.ToAnyAudioChunkPacket) {
	if packet == nil || !recorderReady {
		return
	}

	copied := &network.ToAnyAudioChunkPacket{
		ChunkNumber: packet.ChunkNumber,
		SampleRate:  packet.SampleRate,
		Samples:     append([]int16(nil), packet.Samples...),
	}

	select {
	case recordQueue <- copied:
	default:
		// Drop oldest recording chunk to keep the recorder near real-time.
		select {
		case <-recordQueue:
		default:
		}
		select {
		case recordQueue <- copied:
		default:
		}
	}
}

func stopAudioRecorder() {
	if !recorderReady {
		return
	}
	close(recorderStop)
	recorderWG.Wait()
	close(recordQueue)
	recorderReady = false
}

func handleAudioChunk(packet *network.ToAnyAudioChunkPacket) error {
	if len(packet.Samples) == 0 {
		return nil
	}

	audioStreamsMu.Lock()
	defer audioStreamsMu.Unlock()

	stream, err := getOrCreateNodeAudioStream(routedStreamID, packet.SampleRate)
	if err != nil {
		return err
	}

	if packet.ChunkNumber < stream.expectedChunk {
		return nil
	}
	if _, exists := stream.pending[packet.ChunkNumber]; !exists {
		samples := make([]int16, len(packet.Samples))
		copy(samples, packet.Samples)
		stream.pending[packet.ChunkNumber] = samples
		if stream.chunkSize == 0 {
			stream.chunkSize = len(samples)
		}
	}

	if err := flushContiguousChunks(stream); err != nil {
		return err
	}
	if len(stream.pending) > maxPendingChunks || (!stream.lastProgress.IsZero() && time.Since(stream.lastProgress) > maxChunkGapAge) {
		if err := skipGapAndContinue(stream); err != nil {
			return err
		}
	}

	return nil
}

func getOrCreateNodeAudioStream(peerID uint16, sampleRate uint32) (*nodeAudioStream, error) {
	if stream, ok := audioStreams[peerID]; ok {
		return stream, nil
	}

	fileName := fmt.Sprintf("routed_%s.wav", time.Now().Format("20060102_150405"))
	filePath := filepath.Join(audioOutputDir, fileName)
	file, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}

	if err := writeWAVHeader(file, sampleRate, 0); err != nil {
		_ = file.Close()
		return nil, err
	}

	stream := &nodeAudioStream{
		file:          file,
		sampleRate:    sampleRate,
		expectedChunk: 0,
		pending:       make(map[uint32][]int16),
		lastProgress:  time.Now(),
	}
	audioStreams[peerID] = stream
	fmt.Printf("Recording routed audio stream to %s\n", filePath)
	return stream, nil
}

func flushContiguousChunks(stream *nodeAudioStream) error {
	for {
		samples, ok := stream.pending[stream.expectedChunk]
		if !ok {
			return nil
		}
		if err := writePCMSamples(stream.file, samples); err != nil {
			return err
		}
		stream.bytesWritten += uint32(len(samples) * 2)
		delete(stream.pending, stream.expectedChunk)
		stream.expectedChunk++
		stream.lastProgress = time.Now()
	}
}

func skipGapAndContinue(stream *nodeAudioStream) error {
	if len(stream.pending) == 0 {
		return nil
	}
	if _, ok := stream.pending[stream.expectedChunk]; ok {
		return nil
	}

	keys := make([]int, 0, len(stream.pending))
	for chunk := range stream.pending {
		keys = append(keys, int(chunk))
	}
	sort.Ints(keys)
	nextChunk := uint32(keys[0])
	if nextChunk <= stream.expectedChunk {
		return nil
	}
	gapChunks := nextChunk - stream.expectedChunk
	if gapChunks > maxConcealChunks {
		gapChunks = maxConcealChunks
	}

	silenceSamples := stream.chunkSize
	if silenceSamples <= 0 {
		silenceSamples = 1
	}
	silence := make([]int16, silenceSamples)
	for i := uint32(0); i < gapChunks; i++ {
		if err := writePCMSamples(stream.file, silence); err != nil {
			return err
		}
		stream.bytesWritten += uint32(len(silence) * 2)
		stream.expectedChunk++
		stream.lastProgress = time.Now()
	}

	return flushContiguousChunks(stream)
}

func writePCMSamples(file *os.File, samples []int16) error {
	for _, sample := range samples {
		if err := binary.Write(file, binary.LittleEndian, sample); err != nil {
			return err
		}
	}
	return nil
}

func closeNodeAudioStream(peerID uint16) {
	audioStreamsMu.Lock()
	stream, ok := audioStreams[peerID]
	if ok {
		delete(audioStreams, peerID)
	}
	audioStreamsMu.Unlock()
	if !ok {
		return
	}
	if err := finalizeAndCloseStream(stream); err != nil {
		fmt.Printf("Failed to finalize stream for peer %d: %v\n", peerID, err)
	}
}

func closeAllNodeAudioStreams() {
	audioStreamsMu.Lock()
	streams := make([]*nodeAudioStream, 0, len(audioStreams))
	for peerID, stream := range audioStreams {
		delete(audioStreams, peerID)
		fmt.Printf("Stopping routed stream %d\n", peerID)
		streams = append(streams, stream)
	}
	audioStreamsMu.Unlock()

	for _, stream := range streams {
		if err := finalizeAndCloseStream(stream); err != nil {
			fmt.Printf("Failed to finalize audio stream: %v\n", err)
		}
	}
}

func finalizeAndCloseStream(stream *nodeAudioStream) error {
	if err := writeWAVHeader(stream.file, stream.sampleRate, stream.bytesWritten); err != nil {
		_ = stream.file.Close()
		return err
	}
	return stream.file.Close()
}

func writeWAVHeader(file *os.File, sampleRate uint32, dataSize uint32) error {
	if _, err := file.Seek(0, 0); err != nil {
		return err
	}

	chunkSize := uint32(36) + dataSize
	byteRate := sampleRate * 2 // mono, 16-bit
	blockAlign := uint16(2)
	bitsPerSample := uint16(16)

	if _, err := file.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, chunkSize); err != nil {
		return err
	}
	if _, err := file.Write([]byte("WAVE")); err != nil {
		return err
	}
	if _, err := file.Write([]byte("fmt ")); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, sampleRate); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, byteRate); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, blockAlign); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, bitsPerSample); err != nil {
		return err
	}
	if _, err := file.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(file, binary.LittleEndian, dataSize); err != nil {
		return err
	}
	return nil
}
