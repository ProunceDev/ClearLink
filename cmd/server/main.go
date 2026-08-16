package main

import (
	"clearlink/internal/config"
	"clearlink/internal/models"
	"clearlink/internal/network"
	"clearlink/internal/web"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	peers              = make(map[uint16]*models.NetworkPeer)
	peersMu            sync.RWMutex
	activeSourcePeerID atomic.Uint32
	conn               *net.UDPConn
)

const (
	listenNodeLiveTimeout = 300 * time.Millisecond
	startWindowThreshold  = 5 * time.Second
	switchRSSIDelta       = 1.0
)

type listenSourceCandidate struct {
	peerID uint16
	rssi   float64
	start  time.Time
	score  float64
}

func getNextPeerID() (uint16, uint16) {
	peersMu.RLock()
	defer peersMu.RUnlock()

	var maxID uint16 = 0
	for id := range peers {
		if id > maxID {
			maxID = id
		}
	}
	return maxID + 1, uint16(rand.Intn(65535)) // random secret
}

func main() {
	// Initial setup
	fmt.Println("Starting server. Press Ctrl+C to shutdown.")

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\rShutting down server.")
		os.Exit(0)
	}()

	fmt.Println("Initializing config.")
	_, err := config.LoadConfig("config.ini", models.ApplicationTypeServer)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	go heartbeatLoop()
	go audioRouterLoop()
	startWebServer()

	networkLoop()
}

func startWebServer() {
	adminUsername, _ := config.GetConfigValue("AdminUsername", models.ApplicationTypeServer).(string)
	adminPassword, _ := config.GetConfigValue("AdminPassword", models.ApplicationTypeServer).(string)
	webPort, ok := config.GetConfigValue("WebPort", models.ApplicationTypeServer).(int)
	if !ok || webPort <= 0 {
		webPort = 8080
	}

	panel, err := web.NewServer(web.Options{
		Addr:          fmt.Sprintf("0.0.0.0:%d", webPort),
		AdminUsername: adminUsername,
		AdminPassword: adminPassword,
		GetNodes:      getNodeStatuses,
		SaveConfig:    applyNodeConfigUpdate,
	})
	if err != nil {
		log.Fatalf("Failed to initialize API server: %v", err)
	}
	panel.Start()
}

func getNodeStatuses() []web.NodeStatus {
	peersMu.RLock()
	defer peersMu.RUnlock()

	activeListenPeerID := uint16(activeSourcePeerID.Load())

	nodes := make([]web.NodeStatus, 0, len(peers))
	for _, peer := range peers {
		node := web.NodeStatus{
			PeerID:           peer.PeerID,
			Name:             peer.Name,
			NodeType:         string(peer.NodeType),
			RemoteAddr:       peer.RemoteAddr.String(),
			LastHeartbeat:    peer.LastHeartbeat.Format(time.RFC3339),
			LastHeartbeatAgo: time.Since(peer.LastHeartbeat).Round(time.Second).String(),
			Active:           peer.NodeType == models.ApplicationTypeListen && peer.PeerID == activeListenPeerID,
			Config:           make([]web.NodeConfig, 0, len(peer.Config.Entries)),
		}
		for _, entry := range peer.Config.Entries {
			if peer.NodeType != models.ApplicationTypeUnknown && entry.Type != peer.NodeType {
				continue
			}
			node.Config = append(node.Config, web.NodeConfig{
				Key:   entry.Key,
				Type:  string(entry.Type),
				Value: fmt.Sprintf("%v", entry.Var.Data),
			})
		}
		if peer.HasRSSI {
			rssi := peer.LastRSSI
			node.RSSI = &rssi
		}
		nodes = append(nodes, node)
	}

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].PeerID < nodes[j].PeerID
	})

	return nodes
}

func applyNodeConfigUpdate(peerID uint16, key, applicationType, value string) error {
	fmt.Printf("Applying config update to peer %d: %s:%s = %s\n", peerID, applicationType, key, value)
	peersMu.RLock()
	peer, exists := peers[peerID]
	if !exists {
		peersMu.RUnlock()
		return errors.New("node not found")
	}
	requestedType := models.ApplicationType(applicationType)
	if peer.NodeType != models.ApplicationTypeUnknown && requestedType != peer.NodeType {
		peersMu.RUnlock()
		return errors.New("config type does not match node type")
	}
	var targetEntry *models.ConfigEntry
	for i := range peer.Config.Entries {
		entry := &peer.Config.Entries[i]
		if peer.NodeType != models.ApplicationTypeUnknown && entry.Type != peer.NodeType {
			continue
		}
		if entry.Key == key && string(entry.Type) == applicationType {
			copyEntry := *entry
			targetEntry = &copyEntry
			break
		}
	}
	peersMu.RUnlock()

	if targetEntry == nil {
		return errors.New("config entry not found")
	}

	switch targetEntry.Var.Type {
	case models.EntryTypeInt:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return errors.New("invalid integer value")
		}
		targetEntry.Var.Data = parsed
	case models.EntryTypeString:
		targetEntry.Var.Data = value
	default:
		return errors.New("unsupported config entry type")
	}

	updatePeerConfig(peer, targetEntry)

	peersMu.Lock()
	if currentPeer, ok := peers[peerID]; ok {
		for i := range currentPeer.Config.Entries {
			entry := &currentPeer.Config.Entries[i]
			if entry.Key == targetEntry.Key && entry.Type == targetEntry.Type {
				entry.Var.Data = targetEntry.Var.Data
				break
			}
		}
	}
	peersMu.Unlock()

	return nil
}

func heartbeatLoop() {
	// This function can be used to handle any server-side periodic tasks, such as checking for peer timeouts
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		peersMu.Lock()
		for id, peer := range peers {
			if now.Sub(peer.LastHeartbeat) > 60*time.Second {
				fmt.Printf("Peer %d timed out\n", id)
				termPkt := &network.ToAnyTerminatePacket{Reason: "Heartbeat timeout"}
				payload, _ := termPkt.Marshal()
				hdr := &network.Header{
					Type:       network.PacketTypeToAnyTerminate,
					PeerID:     peer.PeerID,
					PeerSecret: peer.PeerSecret,
				}
				if err := network.SendPacket(conn, peer.RemoteAddr, hdr, payload); err != nil {
					fmt.Printf("Failed to send timeout termination to peer %d: %v\n", id, err)
				}
				delete(peers, id)
			} else if !peer.HasSentConf && now.Sub(peer.LastHeartbeat) > 15*time.Second {
				fmt.Printf("Peer %d has not sent config, disconnecting.\n", id)
				termPkt := &network.ToAnyTerminatePacket{Reason: "Config not received in time"}
				payload, _ := termPkt.Marshal()
				hdr := &network.Header{
					Type:       network.PacketTypeToAnyTerminate,
					PeerID:     peer.PeerID,
					PeerSecret: peer.PeerSecret,
				}
				if err := network.SendPacket(conn, peer.RemoteAddr, hdr, payload); err != nil {
					fmt.Printf("Failed to send config-timeout termination to peer %d: %v\n", id, err)
				}
				delete(peers, id)
			} else {
				hdr := &network.Header{
					Type:       network.PacketTypeToAnyHeartbeat,
					PeerID:     peer.PeerID,
					PeerSecret: peer.PeerSecret,
				}
				if err := network.SendPacket(conn, peer.RemoteAddr, hdr, []byte{}); err != nil {
					fmt.Printf("Failed to send heartbeat to peer %d: %v\n", id, err)
				}
			}
		}
		peersMu.Unlock()
	}
}

func updatePeerConfig(peer *models.NetworkPeer, entry *models.ConfigEntry) {
	updatePkt := &network.ToClientUpdateConfigEntryPacket{Entry: *entry}
	payload, _ := updatePkt.Marshal()

	hdr := &network.Header{
		Type:       network.PacketTypeToClientUpdateConfigEntry,
		PeerID:     peer.PeerID,
		PeerSecret: peer.PeerSecret,
	}
	network.SendPacket(conn, peer.RemoteAddr, hdr, payload)
	peersMu.Lock()
	peer.HasSentConf = false
	peersMu.Unlock()
	fmt.Printf("Sent config update to peer %d at %s: %s:%s = %v\n", peer.PeerID, peer.RemoteAddr, string(entry.Type), entry.Key, entry.Var.Data)
}

func networkLoop() {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", config.GetConfigValue("Port", models.ApplicationTypeServer)))
	if err != nil {
		log.Fatalf("Failed to resolve UDP address: %v", err)
	}

	conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on UDP port: %v", err)
	}
	defer conn.Close()

	fmt.Printf("Server listening on port %d\n", config.GetConfigValue("Port", models.ApplicationTypeServer))

	buf := make([]byte, 8192)
	reassembler := network.NewReassembler()

	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Read error: %v", err)
			continue
		}

		packetBytesList, err := reassembler.Push(remoteAddr, buf[:n])
		if err != nil {
			fmt.Printf("bad packet from %s: %v\n", remoteAddr, err)
			continue
		}

		for _, packetBytes := range packetBytesList {
			hdr, payload, err := network.Unmarshal(packetBytes)
			if err != nil {
				fmt.Printf("bad packet from %s: %v\n", remoteAddr, err)
				continue
			}

			peersMu.RLock()
			peer, exists := peers[hdr.PeerID]
			authValid := exists && peer.PeerSecret == hdr.PeerSecret
			peersMu.RUnlock()
			if hdr.Type != network.PacketTypeToServerHello && !authValid {
				fmt.Printf("Received packet from unknown or unauthorized peer %d at %s\n", hdr.PeerID, remoteAddr)
				termPkt := &network.ToAnyTerminatePacket{Reason: "Unknown or unauthorized peer"}
				terminatePayload, _ := termPkt.Marshal()
				terminateHeader := &network.Header{
					Type: network.PacketTypeToAnyTerminate,
				}
				network.SendPacket(conn, remoteAddr, terminateHeader, terminatePayload)
				continue
			}

			switch hdr.Type {
			case network.PacketTypeToServerHello:
				packet, err := network.UnmarshalToServerHello(payload)
				if err != nil {
					fmt.Printf("Failed to unmarshal ToServerHello packet from %s: %v\n", remoteAddr, err)
					continue
				}
				if packet.ProtocolVersion != network.ProtocolVersion {
					fmt.Printf("Unsupported protocol version from %s: %d\n", remoteAddr, packet.ProtocolVersion)
					termPkt := &network.ToAnyTerminatePacket{Reason: "Unsupported protocol version"}
					terminatePayload, _ := termPkt.Marshal()
					terminateHeader := &network.Header{
						Type: network.PacketTypeToAnyTerminate,
					}
					network.SendPacket(conn, remoteAddr, terminateHeader, terminatePayload)
					continue
				}
				if packet.AuthKey == config.GetConfigValue("AuthKey", models.ApplicationTypeServer) {
					id, secret := getNextPeerID()
					initPkt := &network.ToClientInitPacket{PeerID: id, PeerSecret: secret}
					initPayload, _ := initPkt.Marshal()
					initHeader := &network.Header{
						Type: network.PacketTypeToClientInit,
					}
					network.SendPacket(conn, remoteAddr, initHeader, initPayload)

					peersMu.Lock()
					peers[id] = &models.NetworkPeer{
						PeerID:        id,
						PeerSecret:    secret,
						LastHeartbeat: time.Now(),
						RemoteAddr:    remoteAddr,
						Name:          packet.Name,
						NodeType:      packet.NodeType,
					}
					peersMu.Unlock()
					fmt.Printf("%s node %d initialized for %s with name %s\n", packet.NodeType, id, remoteAddr, packet.Name)
				} else {
					fmt.Printf("Invalid AuthKey from %s\n", remoteAddr)
					termPkt := &network.ToAnyTerminatePacket{Reason: "Invalid AuthKey"}
					terminatePayload, _ := termPkt.Marshal()
					terminateHeader := &network.Header{
						Type: network.PacketTypeToAnyTerminate,
					}
					network.SendPacket(conn, remoteAddr, terminateHeader, terminatePayload)
				}
			case network.PacketTypeToAnyHeartbeat:
				peersMu.Lock()
				if peer, ok := peers[hdr.PeerID]; ok {
					peer.LastHeartbeat = time.Now()
				}
				peersMu.Unlock()
			case network.PacketTypeToServerConfig:
				packet, err := network.UnmarshalToServerConfig(payload)
				if err != nil {
					fmt.Printf("Failed to unmarshal ToServerConfig packet from %s: %v\n", remoteAddr, err)
					continue
				}
				peersMu.Lock()
				if peer, ok := peers[hdr.PeerID]; ok {
					peer.Config = packet.Config
					peer.HasSentConf = true
				}
				peersMu.Unlock()
				fmt.Printf("Received config from peer %d at %s\n", hdr.PeerID, remoteAddr)
			case network.PacketTypeToAnyTerminate:
				termPkt, err := network.UnmarshalToAnyTerminate(payload)
				if err != nil {
					fmt.Printf("Failed to parse terminate packet from %s: %v\n", remoteAddr, err)
				} else {
					fmt.Printf("Received termination from peer %d at %s. Reason: %s\n", hdr.PeerID, remoteAddr, termPkt.Reason)
				}
				peersMu.Lock()
				delete(peers, hdr.PeerID)
				peersMu.Unlock()
			case network.PacketTypeToServerRSSI:
				data, err := network.UnmarshalToServerRSSI(payload)
				if err != nil {
					fmt.Printf("Failed to parse RSSI packet from %s: %v\n", remoteAddr, err)
					continue
				}
				peersMu.Lock()
				if peer, ok := peers[hdr.PeerID]; ok {
					peer.LastRSSI = data.RSSI
					peer.HasRSSI = true
				}
				peersMu.Unlock()
			case network.PacketTypeToAnyAudioChunk:
				data, err := network.UnmarshalToServerAudioChunk(payload)
				if err != nil {
					fmt.Printf("Failed to parse audio chunk packet from %s: %v\n", remoteAddr, err)
					continue
				}
				if len(data.Samples) == 0 {
					wasActive := uint16(activeSourcePeerID.Load()) == hdr.PeerID
					markListenNodeAudioInactive(hdr.PeerID)
					if wasActive {
						activeSourcePeerID.CompareAndSwap(uint32(hdr.PeerID), 0)
						forwardAudioStopToBroadcastNodes(conn, hdr.PeerID, data)
					}
					continue
				}
				markListenNodeAudioActivity(hdr.PeerID)
				forwardAudioToBroadcastNodes(conn, hdr.PeerID, data)
			default:
				fmt.Printf("Unknown type %d from %s\n", hdr.Type, remoteAddr)
			}
		}
	}
}

func audioRouterLoop() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		candidates := make([]listenSourceCandidate, 0)

		peersMu.RLock()
		for _, peer := range peers {
			if peer.NodeType != models.ApplicationTypeListen || !peer.HasRSSI || !peer.HasAudio {
				continue
			}
			if now.Sub(peer.LastAudioAt) > listenNodeLiveTimeout {
				continue
			}
			candidates = append(candidates, listenSourceCandidate{
				peerID: peer.PeerID,
				rssi:   peer.LastRSSI,
				start:  peer.AudioStartedAt,
				score:  calculateListenNodeScore(peer, now),
			})
		}
		peersMu.RUnlock()

		selectedPeerID := chooseAudioSource(candidates)
		previousPeerID := uint16(activeSourcePeerID.Swap(uint32(selectedPeerID)))

		if previousPeerID != selectedPeerID {
			if selectedPeerID == 0 {
				fmt.Println("Audio router: no active listen source")
			} else {
				fmt.Printf("Audio router: now routing listen peer %d\n", selectedPeerID)
			}
		}
	}
}

func calculateListenNodeScore(peer *models.NetworkPeer, now time.Time) float64 {
	ageSeconds := now.Sub(peer.AudioStartedAt).Seconds()
	if ageSeconds < 0 {
		ageSeconds = 0
	}

	return peer.LastRSSI //ageSeconds + peer.LastRSSI
}

func chooseAudioSource(candidates []listenSourceCandidate) uint16 {
	if len(candidates) == 0 {
		return 0
	}

	currentPeerID := uint16(activeSourcePeerID.Load())
	if currentPeerID == 0 {
		return chooseInitialSource(candidates)
	}

	current, hasCurrent := findCandidate(candidates, currentPeerID)
	if !hasCurrent {
		return chooseInitialSource(candidates)
	}

	selected := current
	for _, candidate := range candidates {
		if candidate.peerID == current.peerID {
			continue
		}
		if absDuration(candidate.start.Sub(current.start)) > startWindowThreshold {
			continue
		}
		if candidate.rssi-current.rssi < switchRSSIDelta {
			continue
		}
		if candidate.score > selected.score {
			selected = candidate
		}
	}

	return selected.peerID
}

func chooseInitialSource(candidates []listenSourceCandidate) uint16 {
	if len(candidates) == 0 {
		return 0
	}

	earliestStart := candidates[0].start
	for _, candidate := range candidates[1:] {
		if candidate.start.Before(earliestStart) {
			earliestStart = candidate.start
		}
	}

	selected := listenSourceCandidate{}
	selectedSet := false
	for _, candidate := range candidates {
		if candidate.start.Sub(earliestStart) > startWindowThreshold {
			continue
		}
		if !selectedSet || candidate.score > selected.score {
			selected = candidate
			selectedSet = true
		}
	}

	if !selectedSet {
		return 0
	}

	return selected.peerID
}

func findCandidate(candidates []listenSourceCandidate, peerID uint16) (listenSourceCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.peerID == peerID {
			return candidate, true
		}
	}
	return listenSourceCandidate{}, false
}

func markListenNodeAudioActivity(peerID uint16) {
	now := time.Now()
	peersMu.Lock()
	defer peersMu.Unlock()

	peer, exists := peers[peerID]
	if !exists || peer.NodeType != models.ApplicationTypeListen {
		return
	}

	if !peer.HasAudio {
		peer.HasAudio = true
		peer.AudioStartedAt = now
	}
	peer.LastAudioAt = now
}

func markListenNodeAudioInactive(peerID uint16) {
	peersMu.Lock()
	defer peersMu.Unlock()

	peer, exists := peers[peerID]
	if !exists || peer.NodeType != models.ApplicationTypeListen {
		return
	}

	peer.HasAudio = false
	peer.AudioStartedAt = time.Time{}
	peer.LastAudioAt = time.Time{}
}

func forwardAudioToBroadcastNodes(conn *net.UDPConn, sourcePeerID uint16, packet *network.ToAnyAudioChunkPacket) {
	if uint16(activeSourcePeerID.Load()) != sourcePeerID {
		return
	}

	payload, err := packet.Marshal()
	if err != nil {
		fmt.Printf("Failed to marshal audio chunk from peer %d: %v\n", sourcePeerID, err)
		return
	}

	peersMu.RLock()
	broadcastPeers := make([]*models.NetworkPeer, 0)
	for _, peer := range peers {
		if peer.NodeType != models.ApplicationTypeBroadcast {
			continue
		}
		broadcastPeers = append(broadcastPeers, peer)
	}
	peersMu.RUnlock()

	for _, peer := range broadcastPeers {
		hdr := &network.Header{
			Type:       network.PacketTypeToAnyAudioChunk,
			PeerID:     peer.PeerID,
			PeerSecret: peer.PeerSecret,
		}
		if err := network.SendPacket(conn, peer.RemoteAddr, hdr, payload); err != nil {
			fmt.Printf("Failed to route audio to broadcast peer %d: %v\n", peer.PeerID, err)
		}
	}
}

func forwardAudioStopToBroadcastNodes(conn *net.UDPConn, sourcePeerID uint16, packet *network.ToAnyAudioChunkPacket) {
	payload, err := packet.Marshal()
	if err != nil {
		fmt.Printf("Failed to marshal audio stop packet from peer %d: %v\n", sourcePeerID, err)
		return
	}

	peersMu.RLock()
	broadcastPeers := make([]*models.NetworkPeer, 0)
	for _, peer := range peers {
		if peer.NodeType != models.ApplicationTypeBroadcast {
			continue
		}
		broadcastPeers = append(broadcastPeers, peer)
	}
	peersMu.RUnlock()

	for _, peer := range broadcastPeers {
		hdr := &network.Header{
			Type:       network.PacketTypeToAnyAudioChunk,
			PeerID:     peer.PeerID,
			PeerSecret: peer.PeerSecret,
		}
		if err := network.SendPacket(conn, peer.RemoteAddr, hdr, payload); err != nil {
			fmt.Printf("Failed to route audio stop to broadcast peer %d: %v\n", peer.PeerID, err)
		}
	}
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
