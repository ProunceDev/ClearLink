package main

import (
	"clearlink/internal/config"
	"clearlink/internal/models"
	"clearlink/internal/network"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	conn          *net.UDPConn
	serverPeer    models.NetworkPeer
	isConnected   bool
	discordPlayer *DiscordPlayer
	radioPlayer   *RadioPlayer
)

func scheduleRestartAfterConfigUpdate() {
	go func() {
		time.Sleep(5 * time.Second)
		fmt.Println("Config update received from server. Restarting in 5s.")
		os.Exit(0)
	}()
}

func main() {
	fmt.Println("Starting broadcast client. Press Ctrl+C to shutdown.")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		fmt.Println("\rShutting down client.")
		stopAudioRecorder()
		closeAllNodeAudioStreams()
		if discordPlayer != nil {
			discordPlayer.Close()
		}
		if radioPlayer != nil {
			radioPlayer.Close()
		}
		if conn != nil {
			pkt := &network.ToAnyTerminatePacket{Reason: "Broadcast client shutdown"}
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
	_, err = config.LoadConfig("config.ini", models.ApplicationTypeBroadcast)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if err := ensureAudioOutputDir(); err != nil {
		log.Fatalf("Failed to initialize recordings directory: %v", err)
	}
	startAudioRecorder()

	// Initialize output mode
	broadcastType := config.GetConfigValue("Type", models.ApplicationTypeBroadcast).(string)
	switch broadcastType {
	case "DISCORD":
		botToken := config.GetConfigValue("BotToken", models.ApplicationTypeBroadcast).(string)
		guildID := config.GetConfigValue("GuildID", models.ApplicationTypeBroadcast).(string)
		voiceChannelID := config.GetConfigValue("VoiceChannelID", models.ApplicationTypeBroadcast).(string)

		if botToken == "" || guildID == "" || voiceChannelID == "" {
			log.Fatalf("Discord mode requires BotToken, GuildID, and VoiceChannelID in config")
		}

		dp, err := NewDiscordPlayer(botToken, guildID, voiceChannelID)
		if err != nil {
			log.Fatalf("Failed to initialize Discord player: %v", err)
		}
		discordPlayer = dp
		fmt.Println("Discord player initialized")
	case "RADIO":
		pttPin := config.GetConfigValue("Ptt_Pin", models.ApplicationTypeBroadcast).(int)
		rp, err := NewRadioPlayer(pttPin)
		if err != nil {
			log.Fatalf("Failed to initialize radio player: %v", err)
		}
		radioPlayer = rp
		fmt.Printf("Radio player initialized (PTT GPIO=%d)\n", pttPin)
	default:
		log.Fatalf("Unsupported broadcast Type %q. Supported values: DISCORD, RADIO", broadcastType)
	}

	// Resolve server address
	serverAddrStr := fmt.Sprintf("%s:%d", config.GetConfigValue("ServerAddr", models.ApplicationTypeBroadcast), config.GetConfigValue("ServerPort", models.ApplicationTypeBroadcast))
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

	receiveLoop()
}

func attemptConnect() {
	hello := &network.ToServerHelloPacket{
		AuthKey:         config.GetConfigValue("AuthKey", models.ApplicationTypeBroadcast).(string),
		Name:            config.GetConfigValue("NodeName", models.ApplicationTypeBroadcast).(string),
		ProtocolVersion: network.ProtocolVersion,
		NodeType:        models.ApplicationTypeBroadcast,
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
			closeAllNodeAudioStreams()
			if radioPlayer != nil {
				radioPlayer.NotifyTransmitStop()
			}
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
	buf := make([]byte, 8192)
	reassembler := network.NewReassembler()

	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Op == "read" {
				// Closed by shutdown
				return
			}
			log.Printf("Read error: %v", err)
			continue
		}

		packetBytesList, err := reassembler.Push(serverPeer.RemoteAddr, buf[:n])
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

				network.SendPacket(conn, serverPeer.RemoteAddr, configHeader, initPayload)
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
				closeAllNodeAudioStreams()
				stopAudioRecorder()
				startAudioRecorder()
				if radioPlayer != nil {
					radioPlayer.NotifyTransmitStop()
				}
				isConnected = false
			case network.PacketTypeToAnyAudioChunk:
				audioPkt, err := network.UnmarshalToServerAudioChunk(payload)
				if err != nil {
					fmt.Printf("Failed to parse routed audio packet: %v\n", err)
					continue
				}
				if discordPlayer != nil {
					discordPlayer.SendAudio(audioPkt)
				}
				if radioPlayer != nil {
					radioPlayer.SendAudio(audioPkt)
				}
				queueAudioChunkForRecording(audioPkt)
			case network.PacketTypeToClientUpdateConfigEntry:
				updatePkt, err := network.UnmarshalToClientUpdateConfigEntry(payload)
				if err != nil {
					fmt.Printf("Failed to parse config update packet: %v\n", err)
					continue
				}
				config.UpdateConfigValue(updatePkt.Entry)
				fmt.Printf("Updated config entry from server: %s:%s = %v\n", string(updatePkt.Entry.Type), updatePkt.Entry.Key, updatePkt.Entry.Var.Data)
				scheduleRestartAfterConfigUpdate()

				init := &network.ToServerConfigPacket{Config: *config.Config}
				initPayload, _ := init.Marshal()

				configHeader := &network.Header{
					Type:       network.PacketTypeToServerConfig,
					PeerID:     serverPeer.PeerID,
					PeerSecret: serverPeer.PeerSecret,
				}

				network.SendPacket(conn, serverPeer.RemoteAddr, configHeader, initPayload)
				fmt.Println("Sent config to server.")
			default:
				fmt.Printf("Unknown packet type %d from server\n", hdr.Type)
			}
		}
	}
}
