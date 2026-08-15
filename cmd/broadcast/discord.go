package main

import (
	"clearlink/internal/network"
	"context"
	"fmt"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/hraban/opus"
)

const (
	discordOpusSampleRate  = 48000
	discordOpusChannels    = 1
	discordOpusFrameSize   = 960
	discordOpusBitrate     = 128000
	discordQueueChunks     = 2
	discordMaxFrameBacklog = 2
)

type DiscordPlayer struct {
	session        *discordgo.Session
	voiceConn      *discordgo.VoiceConnection
	opusEncoder    *opus.Encoder
	guildID        string
	voiceChannelID string
	audioQueue     chan []int16
	sampleBuffer   []int16 // Buffer to accumulate samples to valid frame size
	quitChan       chan struct{}
	wg             sync.WaitGroup
	mu             sync.Mutex
	isConnected    bool
}

func NewDiscordPlayer(botToken, guildID, voiceChannelID string) (*DiscordPlayer, error) {
	session, err := discordgo.New("Bot " + botToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create Discord session: %w", err)
	}

	err = session.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open Discord session: %w", err)
	}

	// Discord voice uses 20 ms Opus frames at 48 kHz.
	opusEnc, err := opus.NewEncoder(discordOpusSampleRate, discordOpusChannels, opus.Application(opus.AppVoIP))
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("failed to create Opus encoder: %w", err)
	}
	opusEnc.SetBitrate(discordOpusBitrate)

	dp := &DiscordPlayer{
		session:        session,
		opusEncoder:    opusEnc,
		guildID:        guildID,
		voiceChannelID: voiceChannelID,
		audioQueue:     make(chan []int16, discordQueueChunks),
		sampleBuffer:   make([]int16, 0, discordOpusFrameSize*2),
		quitChan:       make(chan struct{}),
		isConnected:    false,
	}

	// Join voice channel
	voiceConn, err := session.ChannelVoiceJoin(context.Background(), guildID, voiceChannelID, false, true)
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("failed to join voice channel: %w", err)
	}

	dp.voiceConn = voiceConn
	dp.mu.Lock()
	dp.isConnected = true
	dp.mu.Unlock()

	fmt.Printf("Connected to Discord voice channel %s with Opus encoder\n", voiceChannelID)

	dp.wg.Add(1)
	go dp.audioSendLoop()

	return dp, nil
}

func (dp *DiscordPlayer) audioSendLoop() {
	defer dp.wg.Done()

	opusBuffer := make([]byte, 4096)

	for {
		select {
		case <-dp.quitChan:
			return
		case samples, open := <-dp.audioQueue:
			if !open {
				return
			}

			if dp.voiceConn == nil {
				continue
			}

			dp.mu.Lock()
			dp.sampleBuffer = append(dp.sampleBuffer, samples...)
			if len(dp.sampleBuffer) > discordOpusFrameSize*discordMaxFrameBacklog {
				dp.sampleBuffer = append([]int16(nil), dp.sampleBuffer[len(dp.sampleBuffer)-discordOpusFrameSize*discordMaxFrameBacklog:]...)
			}

			for len(dp.sampleBuffer) >= discordOpusFrameSize {
				frame := append([]int16(nil), dp.sampleBuffer[:discordOpusFrameSize]...)
				dp.sampleBuffer = dp.sampleBuffer[discordOpusFrameSize:]
				dp.mu.Unlock()

				encodedLen, err := dp.opusEncoder.Encode(frame, opusBuffer)
				if err != nil {
					fmt.Printf("Opus encoding error: %v\n", err)
					dp.mu.Lock()
					continue
				}

				packet := append([]byte(nil), opusBuffer[:encodedLen]...)

				select {
				case dp.voiceConn.OpusSend <- packet:
				case <-dp.quitChan:
					return
				default:
					select {
					case <-dp.voiceConn.OpusSend:
					default:
					}

					select {
					case dp.voiceConn.OpusSend <- packet:
					case <-dp.quitChan:
						return
					default:
						fmt.Println("Discord relay congested, dropping stale audio")
					}
				}

				dp.mu.Lock()
			}
			dp.mu.Unlock()
		}
	}
}

func (dp *DiscordPlayer) SendAudio(chunk *network.ToAnyAudioChunkPacket) {
	if chunk == nil {
		return
	}
	if len(chunk.Samples) == 0 {
		dp.resetBuffers()
		return
	}

	samples := append([]int16(nil), chunk.Samples...)

	select {
	case dp.audioQueue <- samples:
	case <-dp.quitChan:
	default:
		dp.mu.Lock()
		dp.sampleBuffer = dp.sampleBuffer[:0]
		dp.mu.Unlock()

		select {
		case <-dp.audioQueue:
		default:
		}

		select {
		case dp.audioQueue <- samples:
		case <-dp.quitChan:
		default:
		}
	}
}

func (dp *DiscordPlayer) resetBuffers() {
	dp.mu.Lock()
	dp.sampleBuffer = dp.sampleBuffer[:0]
	dp.mu.Unlock()

	for {
		select {
		case <-dp.audioQueue:
		default:
			return
		}
	}
}

func (dp *DiscordPlayer) Close() error {
	dp.mu.Lock()
	if !dp.isConnected {
		dp.mu.Unlock()
		return nil
	}
	dp.isConnected = false
	dp.mu.Unlock()

	close(dp.quitChan)
	dp.wg.Wait()

	close(dp.audioQueue)

	if dp.voiceConn != nil {
		dp.voiceConn.Disconnect(context.Background())
	}

	if dp.session != nil {
		dp.session.Close()
	}

	fmt.Println("Disconnected from Discord")
	return nil
}

func (dp *DiscordPlayer) IsConnected() bool {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	return dp.isConnected
}
