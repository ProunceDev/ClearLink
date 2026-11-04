package main

import (
	"clearlink/internal/models"
	"net"
	"testing"
	"time"
)

func TestRefreshActiveAudioSourceSelectsEligibleNodeImmediately(t *testing.T) {
	peersMu.Lock()
	originalPeers := peers
	peers = map[uint16]*models.NetworkPeer{
		42: {
			PeerID:         42,
			NodeType:       models.ApplicationTypeListen,
			HasRSSI:        true,
			LastRSSI:       -25,
			HasAudio:       true,
			AudioStartedAt: time.Now(),
			LastAudioAt:    time.Now(),
		},
	}
	peersMu.Unlock()

	originalActive := activeSourcePeerID.Swap(0)
	t.Cleanup(func() {
		activeSourcePeerID.Store(originalActive)
		peersMu.Lock()
		peers = originalPeers
		peersMu.Unlock()
	})

	if selected := refreshActiveAudioSource(); selected != 42 {
		t.Fatalf("refreshActiveAudioSource() = %d, want 42", selected)
	}
	if active := activeSourcePeerID.Load(); active != 42 {
		t.Fatalf("active source = %d, want 42", active)
	}
}

func TestMarkAudioSourceActivityRecordsListenKeyUp(t *testing.T) {
	peersMu.Lock()
	originalPeers := peers
	peers = map[uint16]*models.NetworkPeer{
		42: {
			PeerID:   42,
			NodeType: models.ApplicationTypeListen,
			HasAudio: false,
		},
	}
	peersMu.Unlock()
	t.Cleanup(func() {
		peersMu.Lock()
		peers = originalPeers
		peersMu.Unlock()
	})

	before := time.Now()
	if !markAudioSourceActivity(42) {
		t.Fatal("markAudioSourceActivity() = false, want true")
	}

	peersMu.RLock()
	peer := peers[42]
	hasAudio := peer.HasAudio
	lastKeyUp := peer.LastKeyUp
	peersMu.RUnlock()

	if !hasAudio {
		t.Fatal("active listen node has no audio")
	}
	if lastKeyUp.IsZero() || lastKeyUp.Before(before) {
		t.Fatalf("LastKeyUp = %v, want a time at or after %v", lastKeyUp, before)
	}
}

func TestEnsureNodeConfigDefaultsAddsMissingBroadcastHangTime(t *testing.T) {
	received := models.Config{Entries: []models.ConfigEntry{
		{
			Key:     "NodeName",
			Type:    models.ApplicationTypeBroadcast,
			Var:     models.EntryVar{Type: models.EntryTypeString, Data: "Radio"},
			Default: models.EntryVar{Type: models.EntryTypeString, Data: "DefaultBroadcastNodeName"},
		},
	}}

	normalized := ensureNodeConfigDefaults(models.ApplicationTypeBroadcast, received)
	for _, entry := range normalized.Entries {
		if entry.Key != "HangTimeMS" || entry.Type != models.ApplicationTypeBroadcast {
			continue
		}
		if got, want := entry.Var.Data, 50; got != want {
			t.Fatalf("HangTimeMS = %v, want %v", got, want)
		}
		return
	}
	t.Fatal("normalized broadcast config is missing HangTimeMS")
}

func TestGetNodeStatusesIncludesDefaultBroadcastHangTime(t *testing.T) {
	peersMu.Lock()
	originalPeers := peers
	peers = map[uint16]*models.NetworkPeer{
		7: {
			PeerID:     7,
			Name:       "Radio",
			NodeType:   models.ApplicationTypeBroadcast,
			RemoteAddr: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4125},
			Config: models.Config{Entries: []models.ConfigEntry{
				{
					Key:     "NodeName",
					Type:    models.ApplicationTypeBroadcast,
					Var:     models.EntryVar{Type: models.EntryTypeString, Data: "Radio"},
					Default: models.EntryVar{Type: models.EntryTypeString, Data: "DefaultBroadcastNodeName"},
				},
			}},
		},
	}
	peersMu.Unlock()
	t.Cleanup(func() {
		peersMu.Lock()
		peers = originalPeers
		peersMu.Unlock()
	})

	nodes := getNodeStatuses()
	if len(nodes) != 1 {
		t.Fatalf("node count = %d, want 1", len(nodes))
	}
	for _, entry := range nodes[0].Config {
		if entry.Key != "HangTimeMS" || entry.Type != string(models.ApplicationTypeBroadcast) {
			continue
		}
		if got, want := entry.Value, "50"; got != want {
			t.Fatalf("HangTimeMS value = %q, want %q", got, want)
		}
		return
	}
	t.Fatal("node status is missing HangTimeMS")
}
