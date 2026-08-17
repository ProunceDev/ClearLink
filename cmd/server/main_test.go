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
