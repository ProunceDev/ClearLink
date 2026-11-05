package main

import (
	"clearlink/internal/models"
	"clearlink/internal/web"
	"os"
	"path/filepath"
	"testing"
)

func TestAudioEventManagerPersistsValidatedEvent(t *testing.T) {
	mediaDir := t.TempDir()
	const mediaID = "abcdef0123456789abcdef0123456789"
	if err := os.WriteFile(filepath.Join(mediaDir, mediaID+".mp3"), []byte("ID3test"), 0o644); err != nil {
		t.Fatalf("write MP3 fixture: %v", err)
	}

	manager, err := newAudioEventManager(mediaDir)
	if err != nil {
		t.Fatalf("newAudioEventManager() error = %v", err)
	}
	saved, err := manager.save(web.AudioEvent{
		Name:           "Evening ID",
		Type:           audioEventTypeRepeaterID,
		Enabled:        true,
		MediaIDs:       []string{mediaID, mediaID},
		SourceNodeType: "all",
	})
	if err != nil {
		t.Fatalf("save() error = %v", err)
	}
	if !isAudioEventID(saved.ID) {
		t.Fatalf("saved event ID = %q, want generated ID", saved.ID)
	}
	if saved.IntervalSeconds != int(defaultRepeaterIDInterval.Seconds()) {
		t.Fatalf("interval = %d, want %d", saved.IntervalSeconds, int(defaultRepeaterIDInterval.Seconds()))
	}
	if len(saved.MediaIDs) != 1 || saved.MediaIDs[0] != mediaID {
		t.Fatalf("media IDs = %v, want one selected MP3", saved.MediaIDs)
	}
	if err := os.Remove(filepath.Join(mediaDir, mediaID+".mp3")); err != nil {
		t.Fatalf("remove MP3 fixture: %v", err)
	}

	reloaded, err := newAudioEventManager(mediaDir)
	if err != nil {
		t.Fatalf("reload manager error = %v", err)
	}
	events := reloaded.list()
	if len(events) != 1 || events[0].ID != saved.ID {
		t.Fatalf("reloaded events = %+v, want saved event", events)
	}
	if err := reloaded.delete(saved.ID); err != nil {
		t.Fatalf("delete() error = %v", err)
	}
	if got := reloaded.list(); len(got) != 0 {
		t.Fatalf("events after delete = %+v, want none", got)
	}
}

func TestAudioEventMatchesSourceFilter(t *testing.T) {
	listenSource := audioRouteSource{peerID: 8, nodeType: models.ApplicationTypeListen}
	if !audioEventMatchesSource(web.AudioEvent{SourceNodeType: "all"}, &listenSource) {
		t.Fatal("all-source event should match listen source")
	}
	if !audioEventMatchesSource(web.AudioEvent{SourceNodeType: "listen", SourcePeerID: 8}, &listenSource) {
		t.Fatal("matching listen peer event should match source")
	}
	if audioEventMatchesSource(web.AudioEvent{SourceNodeType: "interface"}, &listenSource) {
		t.Fatal("interface-only event must not match listen source")
	}
	if audioEventMatchesSource(web.AudioEvent{SourceNodeType: "listen", SourcePeerID: 9}, &listenSource) {
		t.Fatal("different peer event must not match source")
	}
}
