package main

import (
	"clearlink/internal/models"
	"clearlink/internal/network"
	"net"
	"testing"
	"time"
)

func TestAudioRoutingSnapshotMixesInterfaceAndSelectedListenSource(t *testing.T) {
	peersMu.Lock()
	originalPeers := peers
	peers = map[uint16]*models.NetworkPeer{
		10: {
			PeerID:      10,
			NodeType:    models.ApplicationTypeInterface,
			HasAudio:    true,
			LastAudioAt: testNow,
			Config:      models.Config{Entries: []models.ConfigEntry{{Key: "RouteToBroadcast", Type: models.ApplicationTypeInterface, Var: models.EntryVar{Type: models.EntryTypeBool, Data: true}}, {Key: "RouteToInterface", Type: models.ApplicationTypeInterface, Var: models.EntryVar{Type: models.EntryTypeBool, Data: false}}}},
		},
		11: {
			PeerID:      11,
			NodeType:    models.ApplicationTypeListen,
			HasAudio:    true,
			LastAudioAt: testNow,
			Config:      models.Config{Entries: []models.ConfigEntry{{Key: "RouteToBroadcast", Type: models.ApplicationTypeListen, Var: models.EntryVar{Type: models.EntryTypeBool, Data: true}}, {Key: "RouteToInterface", Type: models.ApplicationTypeListen, Var: models.EntryVar{Type: models.EntryTypeBool, Data: true}}}},
		},
		12: {PeerID: 12, NodeType: models.ApplicationTypeBroadcast},
	}
	peersMu.Unlock()
	t.Cleanup(func() {
		peersMu.Lock()
		peers = originalPeers
		peersMu.Unlock()
	})

	sources, destinations := audioRoutingSnapshot(testNow, 11)
	if len(sources) != 2 {
		t.Fatalf("audio routing sources = %d, want 2", len(sources))
	}
	if len(destinations) != 2 {
		t.Fatalf("audio routing destinations = %d, want 2", len(destinations))
	}
}

func TestShouldRouteAudioPreventsInterfaceInputLoop(t *testing.T) {
	interfaceSource := audioRouteSource{
		peerID:           7,
		nodeType:         models.ApplicationTypeInterface,
		routeToBroadcast: true,
		routeToInterface: true,
	}
	if shouldRouteAudio(interfaceSource, audioRouteDestination{peerID: 7, nodeType: models.ApplicationTypeInterface}) {
		t.Fatal("interface source must not route into its own output")
	}
	if shouldRouteAudio(interfaceSource, audioRouteDestination{peerID: 8, nodeType: models.ApplicationTypeInterface}) {
		t.Fatal("interface source must not route into any interface output")
	}
	if !shouldRouteAudio(interfaceSource, audioRouteDestination{peerID: 9, nodeType: models.ApplicationTypeBroadcast}) {
		t.Fatal("interface source should route to broadcast when enabled")
	}
}

func TestPeerIsAudioActiveReflectsEligibleDestinationRoute(t *testing.T) {
	peersMu.Lock()
	originalPeers := peers
	peers = map[uint16]*models.NetworkPeer{
		5: {
			PeerID:      5,
			NodeType:    models.ApplicationTypeInterface,
			HasAudio:    true,
			LastAudioAt: testNow,
			Config: models.Config{Entries: []models.ConfigEntry{
				{Key: "RouteToBroadcast", Type: models.ApplicationTypeInterface, Var: models.EntryVar{Type: models.EntryTypeBool, Data: true}},
			}},
		},
		6: {PeerID: 6, NodeType: models.ApplicationTypeBroadcast},
	}
	active := peerIsAudioActiveLocked(peers[6], testNow, 0)
	peersMu.Unlock()
	t.Cleanup(func() {
		peersMu.Lock()
		peers = originalPeers
		peersMu.Unlock()
	})

	if !active {
		t.Fatal("broadcast destination should be active when an eligible interface source is live")
	}
}

func TestMixPCM16SaturatesCombinedSources(t *testing.T) {
	first := make([]int16, audioMixFrameSamples)
	second := make([]int16, audioMixFrameSamples)
	first[0], second[0] = 30000, 30000
	first[1], second[1] = -30000, -30000
	first[2], second[2] = 1000, -400

	mixed := mixPCM16([][]int16{first, second})
	if mixed[0] != 32767 || mixed[1] != -32768 || mixed[2] != 600 {
		t.Fatalf("mixed samples = %v, want [32767 -32768 600]", mixed[:3])
	}
}

func TestMixedAudioRouterResamplesToOutputFrame(t *testing.T) {
	router := newMixedAudioRouter()
	input := make([]int16, 480)
	for index := range input {
		input[index] = 1200
	}
	router.push(4, &network.ToAnyAudioChunkPacket{SampleRate: 24000, Samples: input})

	frames := router.takeFrames([]audioRouteSource{{peerID: 4}})
	frame := frames[4]
	if len(frame) != audioMixFrameSamples {
		t.Fatalf("mixed frame length = %d, want %d", len(frame), audioMixFrameSamples)
	}
	if frame[0] != 1200 || frame[len(frame)-1] != 1200 {
		t.Fatalf("resampled frame edge values = %d, %d, want 1200", frame[0], frame[len(frame)-1])
	}
}

func TestRouteMixedAudioMixesRFAndInterfaceButDoesNotReturnInterfaceInput(t *testing.T) {
	serverConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		t.Fatalf("listen server UDP socket: %v", err)
	}
	defer serverConn.Close()

	broadcastConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		t.Fatalf("listen broadcast UDP socket: %v", err)
	}
	defer broadcastConn.Close()
	interfaceConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		t.Fatalf("listen interface UDP socket: %v", err)
	}
	defer interfaceConn.Close()

	now := time.Now()
	peersMu.Lock()
	originalPeers := peers
	peers = map[uint16]*models.NetworkPeer{
		10: {
			PeerID:      10,
			NodeType:    models.ApplicationTypeInterface,
			HasAudio:    true,
			LastAudioAt: now,
			RemoteAddr:  interfaceConn.LocalAddr().(*net.UDPAddr),
			Config: models.Config{Entries: []models.ConfigEntry{
				{Key: "RouteToBroadcast", Type: models.ApplicationTypeInterface, Var: models.EntryVar{Type: models.EntryTypeBool, Data: true}},
			}},
		},
		11: {
			PeerID:         11,
			NodeType:       models.ApplicationTypeListen,
			HasRSSI:        true,
			LastRSSI:       -25,
			HasAudio:       true,
			AudioStartedAt: now,
			LastAudioAt:    now,
			Config: models.Config{Entries: []models.ConfigEntry{
				{Key: "RouteToBroadcast", Type: models.ApplicationTypeListen, Var: models.EntryVar{Type: models.EntryTypeBool, Data: true}},
				{Key: "RouteToInterface", Type: models.ApplicationTypeListen, Var: models.EntryVar{Type: models.EntryTypeBool, Data: true}},
			}},
		},
		12: {
			PeerID:     12,
			NodeType:   models.ApplicationTypeBroadcast,
			RemoteAddr: broadcastConn.LocalAddr().(*net.UDPAddr),
		},
	}
	peersMu.Unlock()

	originalActive := activeSourcePeerID.Swap(0)
	originalRouter := audioRouter
	audioRouter = newMixedAudioRouter()
	t.Cleanup(func() {
		activeSourcePeerID.Store(originalActive)
		audioRouter = originalRouter
		peersMu.Lock()
		peers = originalPeers
		peersMu.Unlock()
	})

	interfaceSamples := make([]int16, audioMixFrameSamples)
	listenSamples := make([]int16, audioMixFrameSamples)
	for index := range interfaceSamples {
		interfaceSamples[index] = 1000
		listenSamples[index] = 2000
	}
	audioRouter.push(10, &network.ToAnyAudioChunkPacket{SampleRate: audioMixSampleRate, Samples: interfaceSamples})
	audioRouter.push(11, &network.ToAnyAudioChunkPacket{SampleRate: audioMixSampleRate, Samples: listenSamples})

	routeMixedAudio(serverConn)

	if packet := receiveRoutedAudioPacket(t, broadcastConn); packet.Samples[0] != 3000 {
		t.Fatalf("broadcast mixed sample = %d, want 3000", packet.Samples[0])
	}
	if packet := receiveRoutedAudioPacket(t, interfaceConn); packet.Samples[0] != 2000 {
		t.Fatalf("interface output sample = %d, want RF-only 2000", packet.Samples[0])
	}
}

func receiveRoutedAudioPacket(t *testing.T, conn *net.UDPConn) *network.ToAnyAudioChunkPacket {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	reassembler := network.NewReassembler()
	buf := make([]byte, network.MaxDatagramSize)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			t.Fatalf("read routed audio: %v", err)
		}
		packetBytesList, err := reassembler.Push(remoteAddr, buf[:n])
		if err != nil {
			t.Fatalf("reassemble routed audio: %v", err)
		}
		for _, packetBytes := range packetBytesList {
			hdr, payload, err := network.Unmarshal(packetBytes)
			if err != nil {
				t.Fatalf("unmarshal routed header: %v", err)
			}
			if hdr.Type != network.PacketTypeToAnyAudioChunk {
				t.Fatalf("routed header type = %d, want audio", hdr.Type)
			}
			packet, err := network.UnmarshalToServerAudioChunk(payload)
			if err != nil {
				t.Fatalf("unmarshal routed audio: %v", err)
			}
			return packet
		}
	}
}

var testNow = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
