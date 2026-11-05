package main

import (
	"clearlink/internal/config"
	"clearlink/internal/models"
	"clearlink/internal/network"
	"clearlink/internal/version"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const interfaceReconnectDelay = time.Second

var (
	conn       *net.UDPConn
	serverPeer models.NetworkPeer
	peerMu     sync.RWMutex
	connected  atomic.Bool
)

type interfaceSettings struct {
	inputDevice       string
	outputDevice      string
	audioRate         int
	audioChunkMs      int
	squelchPin        int
	squelchActiveHigh bool
	pttPin            int
	pttActiveHigh     bool
}

func main() {
	fmt.Println("Starting interface client. Press Ctrl+C to shutdown.")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if _, err := config.LoadConfig("config.ini", models.ApplicationTypeInterface); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	settings, err := loadInterfaceSettings()
	if err != nil {
		log.Fatalf("Invalid interface configuration: %v", err)
	}
	displayALSAPCMDevices()

	squelch, err := NewSquelchInput(settings.squelchPin, settings.squelchActiveHigh)
	if err != nil {
		log.Fatalf("Failed to initialize squelch GPIO: %v", err)
	}
	defer squelch.Close()

	player, err := NewSoundcardOutput(settings.outputDevice, settings.pttPin, settings.pttActiveHigh)
	if err != nil {
		log.Fatalf("Failed to initialize soundcard output: %v", err)
	}
	defer player.Close()

	serverAddrStr := fmt.Sprintf("%s:%d", config.GetConfigValue("ServerAddr", models.ApplicationTypeInterface), config.GetConfigValue("ServerPort", models.ApplicationTypeInterface))
	remoteAddr, err := net.ResolveUDPAddr("udp", serverAddrStr)
	if err != nil {
		log.Fatalf("Failed to resolve server address: %v", err)
	}
	peerMu.Lock()
	serverPeer.RemoteAddr = remoteAddr
	peerMu.Unlock()

	conn, err = net.ListenUDP("udp", nil)
	if err != nil {
		log.Fatalf("Failed to create UDP connection: %v", err)
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		fmt.Println("\rShutting down interface client.")
		player.NotifyTransmitStop()
		sendTerminate("Interface client shutdown")
		if conn != nil {
			_ = conn.Close()
		}
	}()

	fmt.Printf("Client ready. Connecting to %s ...\n", serverAddrStr)
	attemptConnect()

	go heartbeatLoop(ctx, player)
	go captureAudioLoop(ctx, settings, squelch)

	receiveLoop(player)
}

func loadInterfaceSettings() (interfaceSettings, error) {
	if err := config.ValidateInterfaceConfig(*config.Config); err != nil {
		return interfaceSettings{}, err
	}

	settings := interfaceSettings{
		inputDevice:       config.GetConfigValue("InputDevice", models.ApplicationTypeInterface).(string),
		outputDevice:      config.GetConfigValue("OutputDevice", models.ApplicationTypeInterface).(string),
		audioRate:         config.GetConfigValue("AudioRate", models.ApplicationTypeInterface).(int),
		audioChunkMs:      config.GetConfigValue("AudioChunkMs", models.ApplicationTypeInterface).(int),
		squelchPin:        config.GetConfigValue("Squelch_Pin", models.ApplicationTypeInterface).(int),
		squelchActiveHigh: config.GetConfigValue("Squelch_Active_High", models.ApplicationTypeInterface).(bool),
		pttPin:            config.GetConfigValue("Ptt_Pin", models.ApplicationTypeInterface).(int),
		pttActiveHigh:     config.GetConfigValue("Ptt_Active_High", models.ApplicationTypeInterface).(bool),
	}
	return settings, nil
}

func attemptConnect() {
	remoteAddr := serverRemoteAddr()
	if conn == nil || remoteAddr == nil {
		return
	}

	hello := &network.ToServerHelloPacket{
		AuthKey:         config.GetConfigValue("AuthKey", models.ApplicationTypeInterface).(string),
		Name:            config.GetConfigValue("NodeName", models.ApplicationTypeInterface).(string),
		ProtocolVersion: network.ProtocolVersion,
		NodeType:        models.ApplicationTypeInterface,
		Version:         version.Get(),
	}
	payload, err := hello.Marshal()
	if err != nil {
		log.Printf("Failed to marshal hello: %v", err)
		return
	}
	if err := network.SendPacket(conn, remoteAddr, &network.Header{Type: network.PacketTypeToServerHello}, payload); err != nil {
		log.Printf("Failed to send hello: %v", err)
		return
	}
	fmt.Println("Attempted connection to server, awaiting response.")
}

func heartbeatLoop(ctx context.Context, player *SoundcardOutput) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !connected.Load() {
				attemptConnect()
				continue
			}
			if time.Since(serverLastHeartbeat()) > 60*time.Second {
				fmt.Println("Server heartbeat timeout. Connection lost.")
				sendTerminate("Server heartbeat timeout")
				connected.Store(false)
				player.NotifyTransmitStop()
				continue
			}
			if err := sendToServer(network.PacketTypeToAnyHeartbeat, []byte{}); err != nil {
				log.Printf("Failed to send heartbeat: %v", err)
			}
		}
	}
}

func captureAudioLoop(ctx context.Context, settings interfaceSettings, squelch *SquelchInput) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := captureAudioOnce(ctx, settings, squelch); err != nil && ctx.Err() == nil {
			log.Printf("Soundcard capture stopped: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interfaceReconnectDelay):
		}
	}
}

func captureAudioOnce(ctx context.Context, settings interfaceSettings, squelch *SquelchInput) error {
	args := []string{
		"-q",
		"-D", settings.inputDevice,
		"-f", "S16_LE",
		"-c", "1",
		"-r", strconv.Itoa(settings.audioRate),
		"-B", "40000",
		"-F", "10000",
		"-t", "raw",
	}
	cmd := exec.CommandContext(ctx, "arecord", args...)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open arecord stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start arecord: %w", err)
	}

	readErr := forwardCapturedAudio(ctx, stdout, settings, squelch)
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return captureAudioError(settings.inputDevice, readErr, waitErr)
}

func captureAudioError(inputDevice string, readErr, waitErr error) error {
	if waitErr != nil {
		return fmt.Errorf("arecord failed for InputDevice %q: %w", inputDevice, waitErr)
	}
	if readErr != nil {
		return fmt.Errorf("arecord audio stream for InputDevice %q: %w", inputDevice, readErr)
	}
	return nil
}

func forwardCapturedAudio(ctx context.Context, reader io.Reader, settings interfaceSettings, squelch *SquelchInput) error {
	chunkSamples := settings.audioRate * settings.audioChunkMs / 1000
	rawChunk := make([]byte, chunkSamples*2)
	var chunkNumber uint32
	var squelchOpen bool

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err := io.ReadFull(reader, rawChunk); err != nil {
			return err
		}

		open, err := squelch.IsOpen()
		if err != nil {
			return fmt.Errorf("read squelch GPIO: %w", err)
		}
		if open && !squelchOpen {
			squelchOpen = true
			fmt.Println("Interface squelch open")
		}
		if !open && squelchOpen {
			squelchOpen = false
			fmt.Println("Interface squelch closed")
			sendInterfaceAudioChunk(chunkNumber, settings.audioRate, nil)
			chunkNumber++
		}
		if !open {
			continue
		}

		sendInterfaceAudioChunk(chunkNumber, settings.audioRate, decodePCM16(rawChunk))
		chunkNumber++
	}
}

func decodePCM16(raw []byte) []int16 {
	samples := make([]int16, len(raw)/2)
	for index := range samples {
		samples[index] = int16(binary.LittleEndian.Uint16(raw[index*2:]))
	}
	return samples
}

func sendInterfaceAudioChunk(chunkNumber uint32, sampleRate int, samples []int16) {
	if !connected.Load() {
		return
	}
	packet := &network.ToAnyAudioChunkPacket{
		ChunkNumber: chunkNumber,
		SampleRate:  uint32(sampleRate),
		Samples:     samples,
	}
	payload, err := packet.Marshal()
	if err != nil {
		log.Printf("Failed to marshal interface audio: %v", err)
		return
	}
	if err := sendToServer(network.PacketTypeToAnyAudioChunk, payload); err != nil {
		log.Printf("Failed to send interface audio: %v", err)
	}
}

func receiveLoop(player *SoundcardOutput) {
	buf := make([]byte, 8192)
	reassembler := network.NewReassembler()

	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Op == "read" {
				return
			}
			log.Printf("Read error: %v", err)
			continue
		}

		packetBytesList, err := reassembler.Push(remoteAddr, buf[:n])
		if err != nil {
			fmt.Printf("Bad packet from server: %v\n", err)
			continue
		}
		for _, packetBytes := range packetBytesList {
			hdr, payload, err := network.Unmarshal(packetBytes)
			if err != nil {
				fmt.Printf("Bad packet from server: %v\n", err)
				continue
			}
			handleServerPacket(remoteAddr, hdr, payload, player)
		}
	}
}

func handleServerPacket(remoteAddr *net.UDPAddr, hdr *network.Header, payload []byte, player *SoundcardOutput) {
	switch hdr.Type {
	case network.PacketTypeToClientInit:
		if connected.Load() {
			fmt.Println("Received duplicate init packet - ignoring")
			return
		}
		initPacket, err := network.UnmarshalToClientInit(payload)
		if err != nil {
			fmt.Printf("Failed to parse init packet: %v\n", err)
			return
		}
		peerMu.Lock()
		serverPeer.PeerID = initPacket.PeerID
		serverPeer.PeerSecret = initPacket.PeerSecret
		serverPeer.LastHeartbeat = time.Now()
		serverPeer.RemoteAddr = remoteAddr
		peerMu.Unlock()
		connected.Store(true)

		fmt.Printf("Connected! Assigned PeerID=%d, Secret=%d\n", initPacket.PeerID, initPacket.PeerSecret)
		sendCurrentConfig()
	case network.PacketTypeToAnyHeartbeat:
		peerMu.Lock()
		serverPeer.LastHeartbeat = time.Now()
		peerMu.Unlock()
	case network.PacketTypeToAnyTerminate:
		termPacket, err := network.UnmarshalToAnyTerminate(payload)
		if err != nil {
			fmt.Printf("Failed to parse terminate packet: %v\n", err)
		} else {
			fmt.Printf("Received termination from server. Reason: %s\n", termPacket.Reason)
		}
		connected.Store(false)
		player.NotifyTransmitStop()
	case network.PacketTypeToAnyAudioChunk:
		audioPacket, err := network.UnmarshalToServerAudioChunk(payload)
		if err != nil {
			fmt.Printf("Failed to parse routed audio packet: %v\n", err)
			return
		}
		player.SendAudio(audioPacket)
	case network.PacketTypeToClientUpdateConfigEntry:
		updatePacket, err := network.UnmarshalToClientUpdateConfigEntry(payload)
		if err != nil {
			fmt.Printf("Failed to parse config update packet: %v\n", err)
			return
		}
		config.UpdateConfigValue(updatePacket.Entry)
		fmt.Printf("Updated config entry from server: %s:%s = %v\n", string(updatePacket.Entry.Type), updatePacket.Entry.Key, updatePacket.Entry.Var.Data)
		sendCurrentConfig()
		go func() {
			time.Sleep(5 * time.Second)
			fmt.Println("Config update received from server. Restarting in 5s.")
			os.Exit(0)
		}()
	default:
		fmt.Printf("Unknown packet type %d from server\n", hdr.Type)
	}
}

func sendCurrentConfig() {
	if !connected.Load() {
		return
	}
	packet := &network.ToServerConfigPacket{Config: *config.Config}
	payload, err := packet.Marshal()
	if err != nil {
		log.Printf("Failed to marshal config: %v", err)
		return
	}
	if err := sendToServer(network.PacketTypeToServerConfig, payload); err != nil {
		log.Printf("Failed to send config: %v", err)
		return
	}
	fmt.Println("Sent config to server.")
}

func sendTerminate(reason string) {
	if !connected.Load() {
		return
	}
	packet := &network.ToAnyTerminatePacket{Reason: reason}
	payload, err := packet.Marshal()
	if err != nil {
		return
	}
	_ = sendToServer(network.PacketTypeToAnyTerminate, payload)
}

func sendToServer(packetType uint8, payload []byte) error {
	if conn == nil || !connected.Load() {
		return errors.New("server connection is not established")
	}
	peerMu.RLock()
	hdr := &network.Header{
		Type:       packetType,
		PeerID:     serverPeer.PeerID,
		PeerSecret: serverPeer.PeerSecret,
	}
	remoteAddr := serverPeer.RemoteAddr
	peerMu.RUnlock()
	return network.SendPacket(conn, remoteAddr, hdr, payload)
}

func serverRemoteAddr() *net.UDPAddr {
	peerMu.RLock()
	defer peerMu.RUnlock()
	return serverPeer.RemoteAddr
}

func serverLastHeartbeat() time.Time {
	peerMu.RLock()
	defer peerMu.RUnlock()
	return serverPeer.LastHeartbeat
}
