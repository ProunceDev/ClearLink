package main

import (
	"clearlink/internal/models"
	"clearlink/internal/network"
	"fmt"
	"math"
	"net"
	"sync"
	"time"
)

const (
	audioMixSampleRate    uint32 = 48000
	audioMixFrameDuration        = 20 * time.Millisecond
	audioMixFrameSamples         = int(audioMixSampleRate) / 50
	maxQueuedAudioSamples        = audioMixFrameSamples * 6
)

type audioRouteSource struct {
	peerID           uint16
	nodeType         models.ApplicationType
	routeToBroadcast bool
	routeToInterface bool
}

type audioRouteDestination struct {
	peerID     uint16
	peerSecret uint16
	remoteAddr *net.UDPAddr
	nodeType   models.ApplicationType
}

type audioSourceBuffer struct {
	samples []int16
}

type mixedAudioRouter struct {
	mu                sync.Mutex
	sources           map[uint16]*audioSourceBuffer
	nextChunkNumber   map[uint16]uint32
	destinationActive map[uint16]bool
}

func newMixedAudioRouter() *mixedAudioRouter {
	return &mixedAudioRouter{
		sources:           make(map[uint16]*audioSourceBuffer),
		nextChunkNumber:   make(map[uint16]uint32),
		destinationActive: make(map[uint16]bool),
	}
}

var audioRouter = newMixedAudioRouter()

func (router *mixedAudioRouter) push(sourcePeerID uint16, packet *network.ToAnyAudioChunkPacket) {
	if packet == nil || packet.SampleRate == 0 || len(packet.Samples) == 0 {
		return
	}

	samples := resamplePCM16(packet.Samples, packet.SampleRate, audioMixSampleRate)
	if len(samples) == 0 {
		return
	}

	router.mu.Lock()
	defer router.mu.Unlock()

	source := router.sources[sourcePeerID]
	if source == nil {
		source = &audioSourceBuffer{}
		router.sources[sourcePeerID] = source
	}
	source.samples = append(source.samples, samples...)
	if excess := len(source.samples) - maxQueuedAudioSamples; excess > 0 {
		source.samples = append([]int16(nil), source.samples[excess:]...)
	}
}

func (router *mixedAudioRouter) removeSource(sourcePeerID uint16) {
	router.mu.Lock()
	delete(router.sources, sourcePeerID)
	router.mu.Unlock()
}

func (router *mixedAudioRouter) removePeer(peerID uint16) {
	router.mu.Lock()
	delete(router.sources, peerID)
	delete(router.nextChunkNumber, peerID)
	delete(router.destinationActive, peerID)
	router.mu.Unlock()
}

func (router *mixedAudioRouter) discardInactiveSources(sources []audioRouteSource) {
	activePeerIDs := make(map[uint16]struct{}, len(sources))
	for _, source := range sources {
		activePeerIDs[source.peerID] = struct{}{}
	}

	router.mu.Lock()
	for peerID := range router.sources {
		if _, active := activePeerIDs[peerID]; !active {
			delete(router.sources, peerID)
		}
	}
	router.mu.Unlock()
}

func (router *mixedAudioRouter) takeFrames(sources []audioRouteSource) map[uint16][]int16 {
	frames := make(map[uint16][]int16, len(sources))

	router.mu.Lock()
	defer router.mu.Unlock()

	for _, source := range sources {
		frame := make([]int16, audioMixFrameSamples)
		if buffered := router.sources[source.peerID]; buffered != nil {
			count := min(len(buffered.samples), len(frame))
			copy(frame, buffered.samples[:count])
			buffered.samples = buffered.samples[count:]
		}
		frames[source.peerID] = frame
	}

	return frames
}

func (router *mixedAudioRouter) nextPacket(destinationPeerID uint16, active bool, samples []int16) (*network.ToAnyAudioChunkPacket, bool) {
	router.mu.Lock()
	defer router.mu.Unlock()

	if !active && !router.destinationActive[destinationPeerID] {
		return nil, false
	}

	packet := &network.ToAnyAudioChunkPacket{
		ChunkNumber: router.nextChunkNumber[destinationPeerID],
		SampleRate:  audioMixSampleRate,
		Samples:     samples,
	}
	router.nextChunkNumber[destinationPeerID]++
	router.destinationActive[destinationPeerID] = active
	return packet, true
}

func routeMixedAudio(conn *net.UDPConn) {
	activeListenPeerID := refreshActiveAudioSource()
	sources, destinations := audioRoutingSnapshot(time.Now(), activeListenPeerID)
	audioRouter.discardInactiveSources(sources)
	frames := audioRouter.takeFrames(sources)

	for _, destination := range destinations {
		destinationFrames := make([][]int16, 0, len(sources))
		for _, source := range sources {
			if shouldRouteAudio(source, destination) {
				destinationFrames = append(destinationFrames, frames[source.peerID])
			}
		}

		active := len(destinationFrames) > 0
		mixed := []int16(nil)
		if active {
			mixed = mixPCM16(destinationFrames)
		}
		packet, shouldSend := audioRouter.nextPacket(destination.peerID, active, mixed)
		if !shouldSend {
			continue
		}

		payload, err := packet.Marshal()
		if err != nil {
			fmt.Printf("Failed to marshal mixed audio for peer %d: %v\n", destination.peerID, err)
			continue
		}
		hdr := &network.Header{
			Type:       network.PacketTypeToAnyAudioChunk,
			PeerID:     destination.peerID,
			PeerSecret: destination.peerSecret,
		}
		if err := network.SendPacket(conn, destination.remoteAddr, hdr, payload); err != nil {
			fmt.Printf("Failed to route mixed audio to peer %d: %v\n", destination.peerID, err)
		}
	}
}

func audioRoutingSnapshot(now time.Time, activeListenPeerID uint16) ([]audioRouteSource, []audioRouteDestination) {
	sources := make([]audioRouteSource, 0)
	destinations := make([]audioRouteDestination, 0)

	peersMu.RLock()
	defer peersMu.RUnlock()

	for _, peer := range peers {
		switch peer.NodeType {
		case models.ApplicationTypeListen:
			if peer.PeerID == activeListenPeerID && isAudioLive(peer, now) {
				sources = append(sources, newAudioRouteSource(peer))
			}
		case models.ApplicationTypeInterface:
			if isAudioLive(peer, now) {
				sources = append(sources, newAudioRouteSource(peer))
			}
		}

		if peer.NodeType == models.ApplicationTypeBroadcast || peer.NodeType == models.ApplicationTypeInterface {
			destinations = append(destinations, audioRouteDestination{
				peerID:     peer.PeerID,
				peerSecret: peer.PeerSecret,
				remoteAddr: peer.RemoteAddr,
				nodeType:   peer.NodeType,
			})
		}
	}

	return sources, destinations
}

func newAudioRouteSource(peer *models.NetworkPeer) audioRouteSource {
	routeToInterface := false
	if peer.NodeType == models.ApplicationTypeListen {
		routeToInterface = peerAudioRouteEnabled(peer, "RouteToInterface", true)
	}

	return audioRouteSource{
		peerID:           peer.PeerID,
		nodeType:         peer.NodeType,
		routeToBroadcast: peerAudioRouteEnabled(peer, "RouteToBroadcast", true),
		routeToInterface: routeToInterface,
	}
}

func peerAudioRouteEnabled(peer *models.NetworkPeer, key string, defaultValue bool) bool {
	for _, entry := range peer.Config.Entries {
		if entry.Type != peer.NodeType || entry.Key != key {
			continue
		}
		if value, ok := entry.Var.Data.(bool); ok {
			return value
		}
	}
	return defaultValue
}

func isAudioLive(peer *models.NetworkPeer, now time.Time) bool {
	return peer.HasAudio && !peer.LastAudioAt.IsZero() && now.Sub(peer.LastAudioAt) <= listenNodeLiveTimeout
}

// peersMu must be held by the caller.
func peerHasRoutedAudioLocked(destination *models.NetworkPeer, now time.Time, activeListenPeerID uint16) bool {
	destinationRoute := audioRouteDestination{
		peerID:   destination.PeerID,
		nodeType: destination.NodeType,
	}
	for _, sourcePeer := range peers {
		var sourceEligible bool
		switch sourcePeer.NodeType {
		case models.ApplicationTypeListen:
			sourceEligible = sourcePeer.PeerID == activeListenPeerID && isAudioLive(sourcePeer, now)
		case models.ApplicationTypeInterface:
			sourceEligible = isAudioLive(sourcePeer, now)
		}
		if sourceEligible && shouldRouteAudio(newAudioRouteSource(sourcePeer), destinationRoute) {
			return true
		}
	}
	return false
}

// peersMu must be held by the caller.
func peerIsAudioActiveLocked(peer *models.NetworkPeer, now time.Time, activeListenPeerID uint16) bool {
	switch peer.NodeType {
	case models.ApplicationTypeListen:
		return peer.PeerID == activeListenPeerID && isAudioLive(peer, now)
	case models.ApplicationTypeInterface:
		return isAudioLive(peer, now) || peerHasRoutedAudioLocked(peer, now, activeListenPeerID)
	case models.ApplicationTypeBroadcast:
		return peerHasRoutedAudioLocked(peer, now, activeListenPeerID)
	default:
		return false
	}
}

func shouldRouteAudio(source audioRouteSource, destination audioRouteDestination) bool {
	if source.peerID == destination.peerID {
		return false
	}
	if source.nodeType == models.ApplicationTypeInterface && destination.nodeType == models.ApplicationTypeInterface {
		return false
	}

	switch destination.nodeType {
	case models.ApplicationTypeBroadcast:
		return source.routeToBroadcast
	case models.ApplicationTypeInterface:
		return source.routeToInterface
	default:
		return false
	}
}

func mixPCM16(frames [][]int16) []int16 {
	if len(frames) == 0 {
		return nil
	}

	mixed := make([]int16, audioMixFrameSamples)
	for sampleIndex := range mixed {
		var sum int64
		for _, frame := range frames {
			if sampleIndex < len(frame) {
				sum += int64(frame[sampleIndex])
			}
		}
		if sum > math.MaxInt16 {
			sum = math.MaxInt16
		} else if sum < math.MinInt16 {
			sum = math.MinInt16
		}
		mixed[sampleIndex] = int16(sum)
	}
	return mixed
}

func resamplePCM16(samples []int16, inputRate, outputRate uint32) []int16 {
	if len(samples) == 0 || inputRate == 0 || outputRate == 0 {
		return nil
	}
	if inputRate == outputRate {
		return append([]int16(nil), samples...)
	}

	outputLength := int((uint64(len(samples))*uint64(outputRate) + uint64(inputRate)/2) / uint64(inputRate))
	if outputLength < 1 {
		outputLength = 1
	}
	resampled := make([]int16, outputLength)
	for outputIndex := range resampled {
		position := float64(outputIndex) * float64(inputRate) / float64(outputRate)
		left := int(position)
		if left >= len(samples)-1 {
			resampled[outputIndex] = samples[len(samples)-1]
			continue
		}
		fraction := position - float64(left)
		value := float64(samples[left]) + (float64(samples[left+1])-float64(samples[left]))*fraction
		resampled[outputIndex] = int16(math.Round(value))
	}
	return resampled
}
