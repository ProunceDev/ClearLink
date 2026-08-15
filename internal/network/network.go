package network

import (
	"bytes"
	"clearlink/internal/models"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	HeaderSize      = 7
	ProtocolVersion = 1
	MaxDatagramSize = 1200

	FragmentHeaderSize       = 8
	DefaultReassemblyTimeout = 5 * time.Second

	PacketTypeToServerHello = iota
	PacketTypeToServerConfig
	PacketTypeToServerRSSI
	PacketTypeToClientInit
	PacketTypeToClientUpdateConfigEntry
	PacketTypeToAnyHeartbeat
	PacketTypeToAnyTerminate
	PacketTypeToAnyAudioChunk
	PacketTypeToAnyFragment
)

var nextFragmentMessageID atomic.Uint32

type Header struct {
	Type       uint8 // packet ID / type
	PeerID     uint16
	PeerSecret uint16
	Length     uint16 // payload length only (not including header)
}

// Marshal turns header + payload into []byte ready to send via UDP
func (h *Header) Marshal(payload []byte) ([]byte, error) {
	if len(payload) > 65535 {
		return nil, errors.New("payload too large")
	}
	h.Length = uint16(len(payload))

	buf := bytes.NewBuffer(make([]byte, 0, HeaderSize+len(payload)))

	binary.Write(buf, binary.BigEndian, h.Type)
	binary.Write(buf, binary.BigEndian, h.PeerID)
	binary.Write(buf, binary.BigEndian, h.PeerSecret)
	binary.Write(buf, binary.BigEndian, h.Length)
	buf.Write(payload)

	return buf.Bytes(), nil
}

// Unmarshal parses received UDP data into header + payload slice
func Unmarshal(data []byte) (*Header, []byte, error) {
	if len(data) < HeaderSize {
		return nil, nil, errors.New("packet too short")
	}

	var h Header
	reader := bytes.NewReader(data)

	binary.Read(reader, binary.BigEndian, &h.Type)
	binary.Read(reader, binary.BigEndian, &h.PeerID)
	binary.Read(reader, binary.BigEndian, &h.PeerSecret)
	binary.Read(reader, binary.BigEndian, &h.Length)

	payloadLen := int(h.Length)
	if len(data) < HeaderSize+payloadLen {
		return &h, nil, errors.New("truncated payload")
	}

	payload := data[HeaderSize : HeaderSize+payloadLen]
	return &h, payload, nil
}

type FragmentPayload struct {
	MessageID     uint32
	FragmentIndex uint16
	FragmentCount uint16
	Chunk         []byte
}

func MarshalFragmentPayload(messageID uint32, fragmentIndex, fragmentCount uint16, chunk []byte) ([]byte, error) {
	if fragmentCount == 0 {
		return nil, errors.New("fragment count must be greater than 0")
	}
	if fragmentIndex >= fragmentCount {
		return nil, errors.New("fragment index out of range")
	}
	if len(chunk) == 0 {
		return nil, errors.New("fragment chunk cannot be empty")
	}

	buf := bytes.NewBuffer(make([]byte, 0, FragmentHeaderSize+len(chunk)))
	if err := binary.Write(buf, binary.BigEndian, messageID); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, fragmentIndex); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, fragmentCount); err != nil {
		return nil, err
	}
	if _, err := buf.Write(chunk); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func UnmarshalFragmentPayload(payload []byte) (*FragmentPayload, error) {
	if len(payload) < FragmentHeaderSize {
		return nil, errors.New("fragment payload too short")
	}

	reader := bytes.NewReader(payload)
	var messageID uint32
	var fragmentIndex uint16
	var fragmentCount uint16

	if err := binary.Read(reader, binary.BigEndian, &messageID); err != nil {
		return nil, err
	}
	if err := binary.Read(reader, binary.BigEndian, &fragmentIndex); err != nil {
		return nil, err
	}
	if err := binary.Read(reader, binary.BigEndian, &fragmentCount); err != nil {
		return nil, err
	}

	if fragmentCount == 0 {
		return nil, errors.New("fragment count must be greater than 0")
	}
	if fragmentIndex >= fragmentCount {
		return nil, errors.New("fragment index out of range")
	}

	return &FragmentPayload{
		MessageID:     messageID,
		FragmentIndex: fragmentIndex,
		FragmentCount: fragmentCount,
		Chunk:         payload[FragmentHeaderSize:],
	}, nil
}

func SendPacket(conn *net.UDPConn, remoteAddr *net.UDPAddr, hdr *Header, payload []byte) error {
	if conn == nil {
		return errors.New("udp connection is nil")
	}
	if remoteAddr == nil {
		return errors.New("remote address is nil")
	}

	packetBytes, err := hdr.Marshal(payload)
	if err != nil {
		return err
	}

	if len(packetBytes) <= MaxDatagramSize {
		_, err := conn.WriteToUDP(packetBytes, remoteAddr)
		return err
	}

	maxChunkSize := MaxDatagramSize - HeaderSize - FragmentHeaderSize
	if maxChunkSize <= 0 {
		return errors.New("max datagram size too small for fragmentation")
	}

	fragmentCount := (len(packetBytes) + maxChunkSize - 1) / maxChunkSize
	if fragmentCount > 65535 {
		return errors.New("packet requires too many fragments")
	}

	messageID := nextFragmentMessageID.Add(1)
	if messageID == 0 {
		messageID = nextFragmentMessageID.Add(1)
	}

	for index := 0; index < fragmentCount; index++ {
		start := index * maxChunkSize
		end := start + maxChunkSize
		if end > len(packetBytes) {
			end = len(packetBytes)
		}

		fragmentPayload, err := MarshalFragmentPayload(messageID, uint16(index), uint16(fragmentCount), packetBytes[start:end])
		if err != nil {
			return err
		}

		fragmentHeader := &Header{
			Type:       PacketTypeToAnyFragment,
			PeerID:     hdr.PeerID,
			PeerSecret: hdr.PeerSecret,
		}
		fragmentBytes, err := fragmentHeader.Marshal(fragmentPayload)
		if err != nil {
			return err
		}

		if _, err := conn.WriteToUDP(fragmentBytes, remoteAddr); err != nil {
			return err
		}
	}

	return nil
}

type fragmentAssembly struct {
	createdAt     time.Time
	fragmentCount uint16
	fragments     [][]byte
	receivedCount int
}

type Reassembler struct {
	mu         sync.Mutex
	assemblies map[string]map[uint32]*fragmentAssembly
	timeout    time.Duration
}

func NewReassembler() *Reassembler {
	return &Reassembler{
		assemblies: make(map[string]map[uint32]*fragmentAssembly),
		timeout:    DefaultReassemblyTimeout,
	}
}

func (r *Reassembler) Push(remoteAddr *net.UDPAddr, datagram []byte) ([][]byte, error) {
	hdr, payload, err := Unmarshal(datagram)
	if err != nil {
		return nil, err
	}

	if hdr.Type != PacketTypeToAnyFragment {
		return [][]byte{datagram}, nil
	}

	fragmentPayload, err := UnmarshalFragmentPayload(payload)
	if err != nil {
		return nil, err
	}

	senderKey := "unknown"
	if remoteAddr != nil {
		senderKey = remoteAddr.String()
	}
	assemblyKey := fmt.Sprintf("%s|%d", senderKey, hdr.PeerID)

	now := time.Now()

	r.mu.Lock()
	defer r.mu.Unlock()

	r.cleanupLocked(now)

	senderAssemblies, exists := r.assemblies[assemblyKey]
	if !exists {
		senderAssemblies = make(map[uint32]*fragmentAssembly)
		r.assemblies[assemblyKey] = senderAssemblies
	}

	assembly, exists := senderAssemblies[fragmentPayload.MessageID]
	if !exists || assembly.fragmentCount != fragmentPayload.FragmentCount {
		assembly = &fragmentAssembly{
			createdAt:     now,
			fragmentCount: fragmentPayload.FragmentCount,
			fragments:     make([][]byte, int(fragmentPayload.FragmentCount)),
		}
		senderAssemblies[fragmentPayload.MessageID] = assembly
	}

	fragmentIndex := int(fragmentPayload.FragmentIndex)
	if assembly.fragments[fragmentIndex] == nil {
		chunkCopy := make([]byte, len(fragmentPayload.Chunk))
		copy(chunkCopy, fragmentPayload.Chunk)
		assembly.fragments[fragmentIndex] = chunkCopy
		assembly.receivedCount++
	}

	if assembly.receivedCount < int(assembly.fragmentCount) {
		return nil, nil
	}

	totalLength := 0
	for _, fragment := range assembly.fragments {
		if fragment == nil {
			return nil, nil
		}
		totalLength += len(fragment)
	}

	reassembled := make([]byte, 0, totalLength)
	for _, fragment := range assembly.fragments {
		reassembled = append(reassembled, fragment...)
	}

	delete(senderAssemblies, fragmentPayload.MessageID)
	if len(senderAssemblies) == 0 {
		delete(r.assemblies, assemblyKey)
	}

	return [][]byte{reassembled}, nil
}

func (r *Reassembler) cleanupLocked(now time.Time) {
	for assemblyKey, senderAssemblies := range r.assemblies {
		for messageID, assembly := range senderAssemblies {
			if now.Sub(assembly.createdAt) > r.timeout {
				delete(senderAssemblies, messageID)
			}
		}
		if len(senderAssemblies) == 0 {
			delete(r.assemblies, assemblyKey)
		}
	}
}

type ToServerHelloPacket struct {
	AuthKey         string
	Name            string
	ProtocolVersion uint8
	NodeType        models.ApplicationType
}

func (p *ToServerHelloPacket) Marshal() ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 4+len(p.AuthKey)+len(p.Name)+len(p.NodeType)))
	buf.WriteByte(p.ProtocolVersion)
	buf.WriteByte(uint8(len(p.AuthKey)))
	buf.WriteString(p.AuthKey)
	buf.WriteByte(uint8(len(p.Name)))
	buf.WriteString(p.Name)
	buf.WriteByte(uint8(len(p.NodeType)))
	buf.WriteString(string(p.NodeType))
	return buf.Bytes(), nil
}

func UnmarshalToServerHello(payload []byte) (*ToServerHelloPacket, error) {
	if len(payload) < 1 {
		return nil, errors.New("payload too short for ToServerHelloPacket")
	}

	i := 0

	// ProtocolVersion
	protocolVersion := payload[i]
	i++

	// AuthKey
	authLen := int(payload[i])
	i++
	authKey := string(payload[i : i+authLen])
	i += authLen

	// Name
	nameLen := int(payload[i])
	i++
	name := string(payload[i : i+nameLen])
	i += nameLen

	// NodeType
	nodeTypeLen := int(payload[i])
	i++
	nodeType := string(payload[i : i+nodeTypeLen])

	return &ToServerHelloPacket{
		ProtocolVersion: protocolVersion,
		AuthKey:         authKey,
		Name:            name,
		NodeType:        models.ApplicationType(nodeType),
	}, nil
}

type ToClientInitPacket struct {
	PeerID     uint16
	PeerSecret uint16
}

func (p *ToClientInitPacket) Marshal() ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 4))
	binary.Write(buf, binary.BigEndian, p.PeerID)
	binary.Write(buf, binary.BigEndian, p.PeerSecret)
	return buf.Bytes(), nil
}

func UnmarshalToClientInit(payload []byte) (*ToClientInitPacket, error) {
	if len(payload) < 4 {
		return nil, errors.New("payload too short for ToClientInitPacket")
	}
	reader := bytes.NewReader(payload)
	var peerID uint16
	var peerSecret uint16
	binary.Read(reader, binary.BigEndian, &peerID)
	binary.Read(reader, binary.BigEndian, &peerSecret)
	return &ToClientInitPacket{PeerID: peerID, PeerSecret: peerSecret}, nil
}

type ToServerConfigPacket struct {
	Config models.Config
}

func (p *ToServerConfigPacket) Marshal() ([]byte, error) {
	return json.Marshal(p.Config)
}

func UnmarshalToServerConfig(payload []byte) (*ToServerConfigPacket, error) {
	var config models.Config
	err := json.Unmarshal(payload, &config)
	if err != nil {
		return nil, err
	}

	for i := range config.Entries {
		entry := &config.Entries[i]
		if entry.Var.Type == models.EntryTypeInt {
			// JSON numbers are float64 by default, convert to int
			if num, ok := entry.Var.Data.(float64); ok {
				entry.Var.Data = int(num)
			}
			if num, ok := entry.Default.Data.(float64); ok {
				entry.Default.Data = int(num)
			}
		}
	}
	return &ToServerConfigPacket{Config: config}, nil
}

type ToAnyTerminatePacket struct {
	Reason string
}

func (p *ToAnyTerminatePacket) Marshal() ([]byte, error) {
	return []byte(p.Reason), nil
}

func UnmarshalToAnyTerminate(payload []byte) (*ToAnyTerminatePacket, error) {
	return &ToAnyTerminatePacket{Reason: string(payload)}, nil
}

type ToClientUpdateConfigEntryPacket struct {
	Entry models.ConfigEntry
}

func (p *ToClientUpdateConfigEntryPacket) Marshal() ([]byte, error) {
	return json.Marshal(p.Entry)
}

func UnmarshalToClientUpdateConfigEntry(payload []byte) (*ToClientUpdateConfigEntryPacket, error) {
	var entry models.ConfigEntry
	err := json.Unmarshal(payload, &entry)
	if err != nil {
		return nil, err
	}
	if entry.Var.Type == models.EntryTypeInt {
		if num, ok := entry.Var.Data.(float64); ok {
			entry.Var.Data = int(num)
		}
		if num, ok := entry.Default.Data.(float64); ok {
			entry.Default.Data = int(num)
		}
	}
	return &ToClientUpdateConfigEntryPacket{Entry: entry}, nil
}

type ToServerRSSIPacket struct {
	RSSI float64
}

func (p *ToServerRSSIPacket) Marshal() ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 8))
	if err := binary.Write(buf, binary.BigEndian, p.RSSI); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func UnmarshalToServerRSSI(payload []byte) (*ToServerRSSIPacket, error) {
	if len(payload) < 8 {
		return nil, errors.New("payload too short for ToServerRSSIPacket")
	}
	reader := bytes.NewReader(payload)
	var rssi float64
	if err := binary.Read(reader, binary.BigEndian, &rssi); err != nil {
		return nil, err
	}
	return &ToServerRSSIPacket{RSSI: rssi}, nil
}

type ToAnyAudioChunkPacket struct {
	ChunkNumber uint32
	SampleRate  uint32
	Samples     []int16
}

func (p *ToAnyAudioChunkPacket) Marshal() ([]byte, error) {
	if len(p.Samples) > 65535 {
		return nil, errors.New("audio chunk too large")
	}

	buf := bytes.NewBuffer(make([]byte, 0, 10+len(p.Samples)*2))
	if err := binary.Write(buf, binary.BigEndian, p.ChunkNumber); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, p.SampleRate); err != nil {
		return nil, err
	}
	sampleCount := uint16(len(p.Samples))
	if err := binary.Write(buf, binary.BigEndian, sampleCount); err != nil {
		return nil, err
	}
	for _, sample := range p.Samples {
		if err := binary.Write(buf, binary.BigEndian, sample); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func UnmarshalToServerAudioChunk(payload []byte) (*ToAnyAudioChunkPacket, error) {
	if len(payload) < 10 {
		return nil, errors.New("payload too short for ToAnyAudioChunkPacket")
	}

	reader := bytes.NewReader(payload)
	var chunkNumber uint32
	var sampleRate uint32
	var sampleCount uint16
	if err := binary.Read(reader, binary.BigEndian, &chunkNumber); err != nil {
		return nil, err
	}
	if err := binary.Read(reader, binary.BigEndian, &sampleRate); err != nil {
		return nil, err
	}
	if err := binary.Read(reader, binary.BigEndian, &sampleCount); err != nil {
		return nil, err
	}

	if len(payload) != 10+int(sampleCount)*2 {
		return nil, errors.New("invalid ToAnyAudioChunkPacket payload length")
	}

	samples := make([]int16, sampleCount)
	for i := range samples {
		if err := binary.Read(reader, binary.BigEndian, &samples[i]); err != nil {
			return nil, err
		}
	}

	return &ToAnyAudioChunkPacket{
		ChunkNumber: chunkNumber,
		SampleRate:  sampleRate,
		Samples:     samples,
	}, nil
}
