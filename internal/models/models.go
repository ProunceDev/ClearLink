package models

import (
	"net"
	"time"
)

type Config struct {
	Entries []ConfigEntry
}

type EntryVar struct {
	Type EntryType
	Data any
}

type ConfigEntry struct {
	Key     string
	Type    ApplicationType
	Var     EntryVar
	Default EntryVar
}

type ApplicationType string

const (
	ApplicationTypeUnknown   ApplicationType = "unknown"
	ApplicationTypeBroadcast ApplicationType = "broadcast"
	ApplicationTypeListen    ApplicationType = "listen"
	ApplicationTypeServer    ApplicationType = "server"
)

type EntryType int

const (
	EntryTypeInt    EntryType = 0
	EntryTypeString EntryType = 1
)

type NetworkPeer struct {
	PeerID         uint16
	PeerSecret     uint16
	LastHeartbeat  time.Time
	LastRSSI       float64
	HasRSSI        bool
	HasAudio       bool
	AudioStartedAt time.Time
	LastAudioAt    time.Time
	Config         Config
	RemoteAddr     *net.UDPAddr
	HasSentConf    bool
	Name           string
	NodeType       ApplicationType
}

type AudioSource struct {
	PeerID uint16
	RSSI   float64
}
