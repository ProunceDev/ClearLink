package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildPublicTopologyIncludesInterfaceNodes(t *testing.T) {
	topology := buildPublicTopology([]NodeStatus{
		{PeerID: 1, Name: "RF", NodeType: "listen", Active: true, LastKeyUpAgo: "12s"},
		{PeerID: 2, Name: "Gateway", NodeType: "interface", Active: true},
		{PeerID: 3, Name: "Relay", NodeType: "broadcast", Active: true},
	})

	if len(topology.Listen) != 1 || topology.Listen[0].Name != "RF" {
		t.Fatalf("listen topology = %+v, want RF node", topology.Listen)
	}
	if got, want := topology.Listen[0].LastKeyUpAgo, "12s"; got != want {
		t.Fatalf("listen last keyup = %q, want %q", got, want)
	}
	if len(topology.Interface) != 1 || topology.Interface[0].Name != "Gateway" || !topology.Interface[0].Active {
		t.Fatalf("interface topology = %+v, want active Gateway node", topology.Interface)
	}
	if len(topology.Broadcast) != 1 || topology.Broadcast[0].Name != "Relay" || !topology.Broadcast[0].Active {
		t.Fatalf("broadcast topology = %+v, want Relay node", topology.Broadcast)
	}
}

func TestBuildPublicConnectionsIncludesBothInterfaceDirections(t *testing.T) {
	connections := buildPublicConnections([]NodeStatus{
		{PeerID: 1, Name: "RF", NodeType: "listen", Active: true},
		{PeerID: 2, Name: "Gateway", NodeType: "interface", Active: true},
		{PeerID: 3, Name: "Relay", NodeType: "broadcast", Active: true},
	})

	if len(connections.Connections) != 4 {
		t.Fatalf("connection count = %d, want 4", len(connections.Connections))
	}

	want := map[string]bool{
		"peer-1-to-server": true,
		"peer-2-to-server": true,
		"server-to-peer-2": true,
		"server-to-peer-3": true,
	}
	for _, connection := range connections.Connections {
		if !want[connection.ID] {
			t.Fatalf("unexpected connection %q", connection.ID)
		}
		if connection.Color != "#ef4444" {
			t.Fatalf("connection %q color = %q, want active color", connection.ID, connection.Color)
		}
		delete(want, connection.ID)
	}
	if len(want) != 0 {
		t.Fatalf("missing connections: %v", want)
	}
}

func TestAdminSessionBulkConfigAndMediaLifecycle(t *testing.T) {
	var savedPeerID uint16
	var savedKey, savedType, savedValue string
	server, err := NewServer(Options{
		AdminUsername: "admin",
		AdminPassword: "secret",
		MediaDir:      t.TempDir(),
		GetNodes: func() []NodeStatus {
			return []NodeStatus{
				{
					PeerID:   7,
					Name:     "North Tower",
					NodeType: "listen",
					Config: []NodeConfig{
						{Key: "AudioRate", Type: "listen", Value: "48000", ValueType: "int"},
					},
				},
			}
		},
		SaveConfig: func(peerID uint16, key, applicationType, value string) error {
			savedPeerID = peerID
			savedKey = key
			savedType = applicationType
			savedValue = value
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}

	loginRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/login",
		strings.NewReader(`{"username":"admin","password":"secret"}`),
	)
	loginResponse := httptest.NewRecorder()
	server.handleAdminLogin(loginResponse, loginRequest)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", loginResponse.Code, http.StatusOK)
	}
	cookies := loginResponse.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != adminSessionCookieName || !cookies[0].HttpOnly {
		t.Fatalf("login cookie = %+v, want persistent HTTP-only admin cookie", cookies)
	}
	sessionCookie := cookies[0]

	nodesRequest := httptest.NewRequest(http.MethodGet, "/api/admin/nodes", nil)
	nodesRequest.AddCookie(sessionCookie)
	nodesResponse := httptest.NewRecorder()
	server.handleAdminNodes(nodesResponse, nodesRequest)
	if nodesResponse.Code != http.StatusOK {
		t.Fatalf("session-authenticated nodes status = %d, want %d", nodesResponse.Code, http.StatusOK)
	}

	bulkRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/config/bulk",
		strings.NewReader(`{"nodeType":"listen","key":"AudioRate","value":"44100"}`),
	)
	bulkRequest.AddCookie(sessionCookie)
	bulkResponse := httptest.NewRecorder()
	server.handleAdminBulkConfig(bulkResponse, bulkRequest)
	if bulkResponse.Code != http.StatusOK {
		t.Fatalf("bulk config status = %d, want %d", bulkResponse.Code, http.StatusOK)
	}
	if savedPeerID != 7 || savedKey != "AudioRate" || savedType != "listen" || savedValue != "44100" {
		t.Fatalf("bulk save = (%d, %q, %q, %q), want (7, AudioRate, listen, 44100)", savedPeerID, savedKey, savedType, savedValue)
	}

	uploadBody := &bytes.Buffer{}
	uploadWriter := multipart.NewWriter(uploadBody)
	part, err := uploadWriter.CreateFormFile("file", "dispatch-tone.mp3")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write([]byte("ID3test MP3 data")); err != nil {
		t.Fatalf("write upload body: %v", err)
	}
	if err := uploadWriter.Close(); err != nil {
		t.Fatalf("close upload body: %v", err)
	}

	uploadRequest := httptest.NewRequest(http.MethodPost, "/api/admin/media", uploadBody)
	uploadRequest.Header.Set("Content-Type", uploadWriter.FormDataContentType())
	uploadRequest.AddCookie(sessionCookie)
	uploadResponse := httptest.NewRecorder()
	server.handleAdminMedia(uploadResponse, uploadRequest)
	if uploadResponse.Code != http.StatusCreated {
		t.Fatalf("media upload status = %d, want %d: %s", uploadResponse.Code, http.StatusCreated, uploadResponse.Body.String())
	}
	var uploaded MediaAsset
	if err := json.NewDecoder(uploadResponse.Body).Decode(&uploaded); err != nil {
		t.Fatalf("decode uploaded media: %v", err)
	}
	if uploaded.Name != "dispatch-tone.mp3" || !isValidMediaID(uploaded.ID) {
		t.Fatalf("uploaded media = %+v, want named MP3 with a valid ID", uploaded)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/admin/media", nil)
	listRequest.AddCookie(sessionCookie)
	listResponse := httptest.NewRecorder()
	server.handleAdminMedia(listResponse, listRequest)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("media list status = %d, want %d", listResponse.Code, http.StatusOK)
	}
	var assets []MediaAsset
	if err := json.NewDecoder(listResponse.Body).Decode(&assets); err != nil {
		t.Fatalf("decode media list: %v", err)
	}
	if len(assets) != 1 || assets[0].ID != uploaded.ID {
		t.Fatalf("media assets = %+v, want uploaded asset", assets)
	}

	streamRequest := httptest.NewRequest(http.MethodGet, uploaded.StreamURL, nil)
	streamRequest.AddCookie(sessionCookie)
	streamResponse := httptest.NewRecorder()
	server.handleAdminMediaItem(streamResponse, streamRequest)
	if streamResponse.Code != http.StatusOK || streamResponse.Body.String() != "ID3test MP3 data" {
		t.Fatalf("media stream = (%d, %q), want MP3 content", streamResponse.Code, streamResponse.Body.String())
	}

	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/admin/media/"+uploaded.ID, nil)
	deleteRequest.AddCookie(sessionCookie)
	deleteResponse := httptest.NewRecorder()
	server.handleAdminMediaItem(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("media delete status = %d, want %d", deleteResponse.Code, http.StatusNoContent)
	}
}
