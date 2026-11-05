package main

import (
	"clearlink/internal/web"
	cryptorand "crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/go-mp3"
)

const (
	audioEventMediaDir         = "media"
	audioEventStoreFileName    = "audio-events.json"
	defaultRepeaterIDInterval  = 15 * time.Minute
	minimumRepeaterIDInterval  = time.Minute
	audioEventTypeRepeaterID   = "repeater_id"
	audioEventTypeAnnouncement = "announcement"
	audioEventTypeRogerBeep    = "roger_beep"
	audioEventTypePreRogerBeep = "pre_roger_beep"
	maxAudioEventVolumePercent = 200
)

type audioEventManager struct {
	mu       sync.RWMutex
	mediaDir string
	store    string
	events   []web.AudioEvent

	lastBroadcastActivity time.Time
	normalBroadcastActive bool
	lastSource            audioRouteSource
	hasLastSource         bool
	playing               bool
	lastAnnouncement      map[string]string
}

func newAudioEventManager(mediaDir string) (*audioEventManager, error) {
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		return nil, fmt.Errorf("create audio event directory: %w", err)
	}

	manager := &audioEventManager{
		mediaDir:              mediaDir,
		store:                 filepath.Join(mediaDir, audioEventStoreFileName),
		events:                make([]web.AudioEvent, 0),
		lastBroadcastActivity: time.Now(),
		lastAnnouncement:      make(map[string]string),
	}
	contents, err := os.ReadFile(manager.store)
	if err != nil {
		if os.IsNotExist(err) {
			return manager, nil
		}
		return nil, fmt.Errorf("read audio events: %w", err)
	}
	if len(contents) == 0 {
		return manager, nil
	}
	if err := json.Unmarshal(contents, &manager.events); err != nil {
		return nil, fmt.Errorf("decode audio events: %w", err)
	}
	for index := range manager.events {
		normalized, err := manager.normalize(manager.events[index], false)
		if err != nil {
			return nil, fmt.Errorf("invalid saved audio event %q: %w", manager.events[index].ID, err)
		}
		manager.events[index] = normalized
	}
	return manager, nil
}

func (manager *audioEventManager) list() []web.AudioEvent {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	events := cloneAudioEvents(manager.events)
	sort.Slice(events, func(first, second int) bool {
		return strings.ToLower(events[first].Name) < strings.ToLower(events[second].Name)
	})
	return events
}

func (manager *audioEventManager) save(event web.AudioEvent) (web.AudioEvent, error) {
	normalized, err := manager.normalize(event, true)
	if err != nil {
		return web.AudioEvent{}, err
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()

	if normalized.ID == "" {
		normalized.ID, err = newAudioEventID()
		if err != nil {
			return web.AudioEvent{}, err
		}
	} else if !isAudioEventID(normalized.ID) {
		return web.AudioEvent{}, fmt.Errorf("invalid audio event ID")
	}

	for _, existing := range manager.events {
		if existing.ID != normalized.ID && strings.EqualFold(existing.Name, normalized.Name) {
			return web.AudioEvent{}, fmt.Errorf("an audio event named %q already exists", normalized.Name)
		}
	}

	updated := false
	for index := range manager.events {
		if manager.events[index].ID != normalized.ID {
			continue
		}
		manager.events[index] = cloneAudioEvent(normalized)
		updated = true
		break
	}
	if !updated {
		manager.events = append(manager.events, cloneAudioEvent(normalized))
	}
	if err := manager.writeLocked(); err != nil {
		return web.AudioEvent{}, err
	}
	return cloneAudioEvent(normalized), nil
}

func (manager *audioEventManager) delete(eventID string) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	for index := range manager.events {
		if manager.events[index].ID != eventID {
			continue
		}
		manager.events = append(manager.events[:index], manager.events[index+1:]...)
		return manager.writeLocked()
	}
	return os.ErrNotExist
}

func (manager *audioEventManager) playNow(eventID string) error {
	manager.mu.Lock()
	var event *web.AudioEvent
	for _, candidate := range manager.events {
		if candidate.ID == eventID {
			copy := cloneAudioEvent(candidate)
			event = &copy
			break
		}
	}
	manager.mu.Unlock()

	if event == nil {
		return os.ErrNotExist
	}
	if !manager.startPlayback(*event) {
		return fmt.Errorf("another audio event is already playing")
	}
	return nil
}

func (manager *audioEventManager) startScheduler() {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for now := range ticker.C {
			manager.triggerScheduledEvents(now)
		}
	}()
}

func (manager *audioEventManager) observeNormalBroadcast(
	active bool,
	source *audioRouteSource,
	now time.Time,
) {
	var triggeredEvent *web.AudioEvent

	manager.mu.Lock()
	wasActive := manager.normalBroadcastActive
	manager.normalBroadcastActive = active
	if active {
		manager.lastBroadcastActivity = now
		if source != nil {
			manager.lastSource = *source
			manager.hasLastSource = true
		}
		if !wasActive {
			triggeredEvent = manager.matchingEventLocked(
				audioEventTypePreRogerBeep,
				source,
			)
		}
	} else if wasActive {
		var lastSource *audioRouteSource
		if manager.hasLastSource {
			copy := manager.lastSource
			lastSource = &copy
		}
		triggeredEvent = manager.matchingEventLocked(
			audioEventTypeRogerBeep,
			lastSource,
		)
	}
	manager.mu.Unlock()

	if triggeredEvent != nil {
		manager.startPlayback(*triggeredEvent)
	}
}

func (manager *audioEventManager) recordInjectedBroadcastActivity(now time.Time) {
	manager.mu.Lock()
	manager.lastBroadcastActivity = now
	manager.mu.Unlock()
}

func (manager *audioEventManager) triggerScheduledEvents(now time.Time) {
	manager.mu.Lock()
	if manager.normalBroadcastActive || manager.playing {
		manager.mu.Unlock()
		return
	}

	localNow := now.Local()
	minuteKey := localNow.Format("2006-01-02T15:04")
	var event *web.AudioEvent
	for _, candidate := range manager.events {
		if !candidate.Enabled || candidate.Type != audioEventTypeAnnouncement || candidate.ScheduleAt != localNow.Format("15:04") {
			continue
		}
		if manager.lastAnnouncement[candidate.ID] == minuteKey {
			continue
		}
		copy := cloneAudioEvent(candidate)
		event = &copy
		break
	}
	if event == nil {
		candidates := make([]web.AudioEvent, 0)
		for _, candidate := range manager.events {
			if !candidate.Enabled || candidate.Type != audioEventTypeRepeaterID {
				continue
			}
			if now.Sub(manager.lastBroadcastActivity) >= time.Duration(candidate.IntervalSeconds)*time.Second {
				candidates = append(candidates, cloneAudioEvent(candidate))
			}
		}
		if len(candidates) > 0 {
			selected := candidates[rand.Intn(len(candidates))]
			event = &selected
		}
	}
	manager.mu.Unlock()

	if event == nil || !manager.startPlayback(*event) {
		return
	}
	if event.Type == audioEventTypeAnnouncement {
		manager.mu.Lock()
		manager.lastAnnouncement[event.ID] = minuteKey
		manager.mu.Unlock()
	}
}

func (manager *audioEventManager) matchingEventLocked(
	eventType string,
	source *audioRouteSource,
) *web.AudioEvent {
	candidates := make([]web.AudioEvent, 0)
	for _, candidate := range manager.events {
		if !candidate.Enabled || candidate.Type != eventType || !audioEventMatchesSource(candidate, source) {
			continue
		}
		candidates = append(candidates, cloneAudioEvent(candidate))
	}
	if len(candidates) == 0 {
		return nil
	}
	selected := candidates[rand.Intn(len(candidates))]
	return &selected
}

func audioEventMatchesSource(event web.AudioEvent, source *audioRouteSource) bool {
	if source == nil {
		return event.SourcePeerID == 0 && event.SourceNodeType == "all"
	}
	if event.SourcePeerID != 0 && event.SourcePeerID != source.peerID {
		return false
	}
	return event.SourceNodeType == "all" || event.SourceNodeType == string(source.nodeType)
}

func (manager *audioEventManager) startPlayback(event web.AudioEvent) bool {
	manager.mu.Lock()
	if manager.playing || len(event.MediaIDs) == 0 {
		manager.mu.Unlock()
		return false
	}
	mediaID := event.MediaIDs[rand.Intn(len(event.MediaIDs))]
	manager.playing = true
	manager.lastBroadcastActivity = time.Now()
	manager.mu.Unlock()

	audioRouter.setInjectedTarget(event.OutputNodeType, event.OutputPeerID)
	audioRouter.beginInjectedAudio()
	go func() {
		if err := manager.playMedia(mediaID, event.VolumePercent); err != nil {
			log.Printf("Audio event %q playback failed: %v", event.Name, err)
		}
		manager.mu.Lock()
		manager.playing = false
		manager.mu.Unlock()
	}()
	return true
}

func (manager *audioEventManager) playMedia(mediaID string, volumePercent int) error {
	gain := float64(volumePercent) / 100
	if volumePercent <= 0 {
		gain = 1
	}
	done := make(chan struct{})
	defer func() {
		audioRouter.finishInjectedAudio(done)
		<-done
	}()

	file, err := os.Open(filepath.Join(manager.mediaDir, mediaID+".mp3"))
	if err != nil {
		return err
	}
	defer file.Close()

	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		return fmt.Errorf("decode MP3: %w", err)
	}
	sampleRate := uint32(decoder.SampleRate())
	if sampleRate == 0 {
		return fmt.Errorf("MP3 has no sample rate")
	}

	decoded := make([]byte, 8192)
	pendingBytes := make([]byte, 0, len(decoded)+4)
	pendingSamples := make([]int16, 0, audioMixFrameSamples*2)
	queuedFrame := false
	for {
		count, readErr := decoder.Read(decoded)
		if count > 0 {
			pendingBytes = append(pendingBytes, decoded[:count]...)
			usableBytes := len(pendingBytes) / 4 * 4
			if usableBytes > 0 {
				stereo := decodeStereoPCM16(pendingBytes[:usableBytes])
				pendingSamples = append(pendingSamples, applyPCM16Gain(resamplePCM16(stereo, sampleRate, audioMixSampleRate), gain)...)
				pendingBytes = append([]byte(nil), pendingBytes[usableBytes:]...)
			}
			for len(pendingSamples) >= audioMixFrameSamples {
				audioRouter.enqueueInjectedFrame(pendingSamples[:audioMixFrameSamples])
				pendingSamples = pendingSamples[audioMixFrameSamples:]
				queuedFrame = true
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if len(pendingSamples) > 0 {
		frame := make([]int16, audioMixFrameSamples)
		copy(frame, pendingSamples)
		audioRouter.enqueueInjectedFrame(frame)
		queuedFrame = true
	}
	if !queuedFrame {
		return fmt.Errorf("MP3 did not contain playable audio")
	}
	return nil
}

func applyPCM16Gain(samples []int16, gain float64) []int16 {
	if gain == 1 {
		return samples
	}
	for index, sample := range samples {
		scaled := math.Round(float64(sample) * gain)
		switch {
		case scaled > math.MaxInt16:
			scaled = math.MaxInt16
		case scaled < math.MinInt16:
			scaled = math.MinInt16
		}
		samples[index] = int16(scaled)
	}
	return samples
}

func decodeStereoPCM16(contents []byte) []int16 {
	mono := make([]int16, 0, len(contents)/4)
	for offset := 0; offset+3 < len(contents); offset += 4 {
		left := int16(binary.LittleEndian.Uint16(contents[offset:]))
		right := int16(binary.LittleEndian.Uint16(contents[offset+2:]))
		mono = append(mono, int16((int32(left)+int32(right))/2))
	}
	return mono
}

func (manager *audioEventManager) normalize(
	event web.AudioEvent,
	requireExistingMedia bool,
) (web.AudioEvent, error) {
	event.ID = strings.TrimSpace(event.ID)
	event.Name = strings.TrimSpace(event.Name)
	event.Type = strings.ToLower(strings.TrimSpace(event.Type))
	event.SourceNodeType = strings.ToLower(strings.TrimSpace(event.SourceNodeType))
	event.ScheduleAt = strings.TrimSpace(event.ScheduleAt)
	if event.Name == "" {
		return web.AudioEvent{}, fmt.Errorf("event name is required")
	}
	if !isAudioEventType(event.Type) {
		return web.AudioEvent{}, fmt.Errorf("invalid audio event type")
	}
	if event.SourceNodeType == "" {
		event.SourceNodeType = "all"
	}
	if event.SourceNodeType != "all" && event.SourceNodeType != "listen" && event.SourceNodeType != "interface" {
		return web.AudioEvent{}, fmt.Errorf("invalid source node type")
	}
	event.OutputNodeType = strings.ToLower(strings.TrimSpace(event.OutputNodeType))
	if event.OutputNodeType == "" {
		event.OutputNodeType = "all"
	}
	if event.OutputNodeType != "all" && event.OutputNodeType != "broadcast" && event.OutputNodeType != "interface" {
		return web.AudioEvent{}, fmt.Errorf("invalid output node type")
	}
	if event.VolumePercent == 0 {
		event.VolumePercent = 100
	}
	if event.VolumePercent < 0 || event.VolumePercent > maxAudioEventVolumePercent {
		return web.AudioEvent{}, fmt.Errorf("volume must be between 1 and %d percent", maxAudioEventVolumePercent)
	}

	event.MediaIDs = uniqueAudioMediaIDs(event.MediaIDs)
	if len(event.MediaIDs) == 0 {
		return web.AudioEvent{}, fmt.Errorf("select at least one MP3 file")
	}
	for _, mediaID := range event.MediaIDs {
		if !isAudioEventID(mediaID) {
			return web.AudioEvent{}, fmt.Errorf("invalid media ID")
		}
		if !requireExistingMedia {
			continue
		}
		if _, err := os.Stat(filepath.Join(manager.mediaDir, mediaID+".mp3")); err != nil {
			if os.IsNotExist(err) {
				return web.AudioEvent{}, fmt.Errorf("selected MP3 file is no longer available")
			}
			return web.AudioEvent{}, fmt.Errorf("check selected MP3 file: %w", err)
		}
	}

	switch event.Type {
	case audioEventTypeRepeaterID:
		if event.IntervalSeconds <= 0 {
			event.IntervalSeconds = int(defaultRepeaterIDInterval.Seconds())
		}
		if time.Duration(event.IntervalSeconds)*time.Second < minimumRepeaterIDInterval {
			return web.AudioEvent{}, fmt.Errorf("repeater ID interval must be at least one minute")
		}
		event.ScheduleAt = ""
	case audioEventTypeAnnouncement:
		if _, err := time.Parse("15:04", event.ScheduleAt); err != nil {
			return web.AudioEvent{}, fmt.Errorf("announcement time must use HH:MM")
		}
		event.IntervalSeconds = 0
	default:
		event.IntervalSeconds = 0
		event.ScheduleAt = ""
	}

	return event, nil
}

func (manager *audioEventManager) writeLocked() error {
	contents, err := json.Marshal(manager.events)
	if err != nil {
		return fmt.Errorf("encode audio events: %w", err)
	}
	temporaryFile, err := os.CreateTemp(manager.mediaDir, ".audio-events-*")
	if err != nil {
		return fmt.Errorf("create audio event store: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporaryFile.Write(contents); err != nil {
		_ = temporaryFile.Close()
		return fmt.Errorf("write audio event store: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close audio event store: %w", err)
	}
	if err := os.Rename(temporaryPath, manager.store); err != nil {
		return fmt.Errorf("save audio event store: %w", err)
	}
	return nil
}

func isAudioEventType(eventType string) bool {
	switch eventType {
	case audioEventTypeRepeaterID, audioEventTypeAnnouncement, audioEventTypeRogerBeep, audioEventTypePreRogerBeep:
		return true
	default:
		return false
	}
}

func uniqueAudioMediaIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func isAudioEventID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, character := range id {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func newAudioEventID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := cryptorand.Read(bytes); err != nil {
		return "", fmt.Errorf("create audio event ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func cloneAudioEvent(event web.AudioEvent) web.AudioEvent {
	event.MediaIDs = append([]string(nil), event.MediaIDs...)
	return event
}

func cloneAudioEvents(events []web.AudioEvent) []web.AudioEvent {
	cloned := make([]web.AudioEvent, len(events))
	for index := range events {
		cloned[index] = cloneAudioEvent(events[index])
	}
	return cloned
}
