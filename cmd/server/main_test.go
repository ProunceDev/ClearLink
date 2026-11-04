package main

import (
	"clearlink/internal/models"
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
