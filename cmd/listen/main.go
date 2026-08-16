// cmd/listen/main.go
package main

import (
	"clearlink/internal/config"
	"clearlink/internal/models"
	"clearlink/internal/network"
	sdrhelper "clearlink/internal/sdr_helper"
	"context"
	"fmt"
	"log"
	"net"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-audio/audio"
)

var (
	conn        *net.UDPConn
	serverPeer  models.NetworkPeer
	isConnected bool
)

func sdrLoop(ctx context.Context) {
	dataChan := make(chan sdrhelper.SDRData, 1)
	var chunkNumber uint32
	var squelchOpen bool
	var dropNextChunk bool

	go sdrhelper.SdrLoop(ctx, dataChan) // Run SDR in background goroutine

	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-dataChan:
			if !ok {
				return
			}
			audioSamples := make([]int, len(data.AudioChunk))
			for i, sample := range data.AudioChunk {
				audioSamples[i] = int(sample)
			}
			_ = &audio.IntBuffer{
				Format:         &audio.Format{NumChannels: 1, SampleRate: data.SampleRate},
				SourceBitDepth: 16,
				Data:           audioSamples,
			}

			squelchDB, _ := config.GetConfigValue("SquelchDB", models.ApplicationTypeListen).(int)
			aboveSquelch := data.RSSI >= float64(squelchDB)
			if aboveSquelch && !squelchOpen {
				squelchOpen = true
				dropNextChunk = true
				fmt.Printf("Squelch open (RSSI %.1f dB >= %d dB)\n", data.RSSI, squelchDB)
			} else if !aboveSquelch && squelchOpen {
				squelchOpen = false
				dropNextChunk = false
				fmt.Printf("Squelch closed (RSSI %.1f dB < %d dB)\n", data.RSSI, squelchDB)
				if isConnected {
					audioPkt := &network.ToAnyAudioChunkPacket{
						ChunkNumber: chunkNumber,
						SampleRate:  uint32(data.SampleRate),
						Samples:     nil,
					}
					audioPayload, err := audioPkt.Marshal()
					if err == nil {
						audioHdr := &network.Header{
							Type:       network.PacketTypeToAnyAudioChunk,
							PeerID:     serverPeer.PeerID,
							PeerSecret: serverPeer.PeerSecret,
						}
						network.SendPacket(conn, serverPeer.RemoteAddr, audioHdr, audioPayload)
						chunkNumber++
					}
				}
			}

			if isConnected {
				if squelchOpen {
					if dropNextChunk {
						dropNextChunk = false
					} else {
						audioPkt := &network.ToAnyAudioChunkPacket{
							ChunkNumber: chunkNumber,
							SampleRate:  uint32(data.SampleRate),
							Samples:     data.AudioChunk,
						}
						audioPayload, err := audioPkt.Marshal()
						if err == nil {
							audioHdr := &network.Header{
								Type:       network.PacketTypeToAnyAudioChunk,
								PeerID:     serverPeer.PeerID,
								PeerSecret: serverPeer.PeerSecret,
							}
							network.SendPacket(conn, serverPeer.RemoteAddr, audioHdr, audioPayload)
							chunkNumber++
						}
					}
				}

				pkt := &network.ToServerRSSIPacket{RSSI: data.RSSI}
				payload, _ := pkt.Marshal()
				hdr := &network.Header{
					Type:       network.PacketTypeToServerRSSI,
					PeerID:     serverPeer.PeerID,
					PeerSecret: serverPeer.PeerSecret,
				}
				network.SendPacket(conn, serverPeer.RemoteAddr, hdr, payload)
			}
		}
	}
}

func main() {
	fmt.Println("Starting listener client. Press Ctrl+C to shutdown.")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		fmt.Println("\rShutting down client.")
		if conn != nil {
			pkt := &network.ToAnyTerminatePacket{Reason: "Listen client shutdown"}
			payload, _ := pkt.Marshal()
			hdr := &network.Header{
				Type:       network.PacketTypeToAnyTerminate,
				PeerID:     serverPeer.PeerID,
				PeerSecret: serverPeer.PeerSecret,
			}
			network.SendPacket(conn, serverPeer.RemoteAddr, hdr, payload)
			conn.Close()
		}

	}()

	// Load config
	var err error
	_, err = config.LoadConfig("config.ini", models.ApplicationTypeListen)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := installRTLSDRRules(); err != nil {
		log.Printf("Warning: RTL-SDR udev rules were not installed automatically.\n%v", err)
	}

	// Resolve server address
	serverAddrStr := fmt.Sprintf("%s:%d", config.GetConfigValue("ServerAddr", models.ApplicationTypeListen), config.GetConfigValue("ServerPort", models.ApplicationTypeListen))
	serverPeer.RemoteAddr, err = net.ResolveUDPAddr("udp", serverAddrStr)
	if err != nil {
		log.Fatalf("Failed to resolve server address: %v", err)
	}

	conn, err = net.ListenUDP("udp", nil)
	if err != nil {
		log.Fatalf("Failed to create UDP connection: %v", err)
	}
	defer conn.Close()

	fmt.Printf("Client ready. Connecting to %s ...\n", serverAddrStr)

	attemptConnect()

	go heartbeatLoop()

	go sdrLoop(ctx)

	receiveLoop()
}

func attemptConnect() {
	hello := &network.ToServerHelloPacket{
		AuthKey:         config.GetConfigValue("AuthKey", models.ApplicationTypeListen).(string),
		Name:            config.GetConfigValue("NodeName", models.ApplicationTypeListen).(string),
		ProtocolVersion: network.ProtocolVersion,
		NodeType:        models.ApplicationTypeListen,
	}

	payload, err := hello.Marshal()
	if err != nil {
		log.Fatalf("Failed to marshal hello: %v", err)
	}

	hdr := &network.Header{
		Type: network.PacketTypeToServerHello,
		// PeerID and PeerSecret are 0 for initial hello
	}
	network.SendPacket(conn, serverPeer.RemoteAddr, hdr, payload)

	fmt.Println("Attempted connection to server, awaiting response.")
}

func heartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if !isConnected {
			attemptConnect()
			continue
		}

		if time.Since(serverPeer.LastHeartbeat) > 60*time.Second {
			fmt.Println("Server heartbeat timeout. Connection lost.")
			pkt := &network.ToAnyTerminatePacket{Reason: "Server heartbeat timeout"}
			payload, _ := pkt.Marshal()
			hdr := &network.Header{
				Type:       network.PacketTypeToAnyTerminate,
				PeerID:     serverPeer.PeerID,
				PeerSecret: serverPeer.PeerSecret,
			}
			network.SendPacket(conn, serverPeer.RemoteAddr, hdr, payload)
			isConnected = false
			continue
		}

		hdr := &network.Header{
			Type:       network.PacketTypeToAnyHeartbeat,
			PeerID:     serverPeer.PeerID,
			PeerSecret: serverPeer.PeerSecret,
		}
		network.SendPacket(conn, serverPeer.RemoteAddr, hdr, []byte{})
	}
}

// receiveLoop handles incoming packets from the server
func receiveLoop() {
	buf := make([]byte, 2048)
	reassembler := network.NewReassembler()

	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Op == "read" {
				// Closed by shutdown
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

			switch hdr.Type {
			case network.PacketTypeToClientInit:
				if isConnected {
					fmt.Println("Received duplicate init packet — ignoring")
					continue
				}

				initPkt, err := network.UnmarshalToClientInit(payload)
				if err != nil {
					fmt.Printf("Failed to parse init packet: %v\n", err)
					continue
				}

				serverPeer.PeerID = initPkt.PeerID
				serverPeer.PeerSecret = initPkt.PeerSecret
				serverPeer.LastHeartbeat = time.Now()
				isConnected = true

				fmt.Printf("Connected! Assigned PeerID=%d, Secret=%d\n", serverPeer.PeerID, serverPeer.PeerSecret)

				init := &network.ToServerConfigPacket{Config: *config.Config}
				initPayload, _ := init.Marshal()

				configHeader := &network.Header{
					Type:       network.PacketTypeToServerConfig,
					PeerID:     serverPeer.PeerID,
					PeerSecret: serverPeer.PeerSecret,
				}
				network.SendPacket(conn, remoteAddr, configHeader, initPayload)
				fmt.Println("Sent config to server.")
			case network.PacketTypeToAnyHeartbeat:
				serverPeer.LastHeartbeat = time.Now()
			case network.PacketTypeToAnyTerminate:
				termPkt, err := network.UnmarshalToAnyTerminate(payload)
				if err != nil {
					fmt.Printf("Failed to parse terminate packet: %v\n", err)
				} else {
					fmt.Printf("Received termination from server. Reason: %s\n", termPkt.Reason)
				}
				isConnected = false
			case network.PacketTypeToClientUpdateConfigEntry:
				updatePkt, err := network.UnmarshalToClientUpdateConfigEntry(payload)
				if err != nil {
					fmt.Printf("Failed to parse config update packet: %v\n", err)
					continue
				}
				config.UpdateConfigValue(updatePkt.Entry)
				fmt.Printf("Updated config entry from server: %s:%s = %v\n", string(updatePkt.Entry.Type), updatePkt.Entry.Key, updatePkt.Entry.Var.Data)

				init := &network.ToServerConfigPacket{Config: *config.Config}
				initPayload, _ := init.Marshal()

				configHeader := &network.Header{
					Type:       network.PacketTypeToServerConfig,
					PeerID:     serverPeer.PeerID,
					PeerSecret: serverPeer.PeerSecret,
				}

				network.SendPacket(conn, remoteAddr, configHeader, initPayload)
				fmt.Println("Sent config to server.")
			default:
				fmt.Printf("Unknown packet type %d from server\n", hdr.Type)
			}
		}
	}
}
