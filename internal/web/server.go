package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type NodeConfig struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type NodeStatus struct {
	PeerID           uint16       `json:"peerId"`
	Name             string       `json:"name"`
	NodeType         string       `json:"nodeType"`
	RemoteAddr       string       `json:"remoteAddr"`
	LastHeartbeat    string       `json:"lastHeartbeat"`
	LastHeartbeatAgo string       `json:"lastHeartbeatAgo"`
	Active           bool         `json:"active"`
	RSSI             *float64     `json:"rssi"`
	Config           []NodeConfig `json:"config"`
}

type publicNode struct {
	PeerID uint16   `json:"peerId"`
	Name   string   `json:"name"`
	Active bool     `json:"active"`
	RSSI   *float64 `json:"rssi"`
}

type publicTopology struct {
	Listen    []publicNode `json:"listen"`
	Server    publicNode   `json:"server"`
	Broadcast []publicNode `json:"broadcast"`
}

type publicConnection struct {
	ID           string  `json:"id"`
	FromNodeID   string  `json:"fromNodeId"`
	ToNodeID     string  `json:"toNodeId"`
	Color        string  `json:"color"`
	Width        float64 `json:"width"`
	Opacity      float64 `json:"opacity"`
	Curvature    float64 `json:"curvature"`
	StartOffsetY float64 `json:"startOffsetY"`
	EndOffsetY   float64 `json:"endOffsetY"`
	DashArray    string  `json:"dashArray"`
}

type publicConnectionsResponse struct {
	Connections []publicConnection `json:"connections"`
}

type Options struct {
	Addr             string
	AdminUsername    string
	AdminPassword    string
	GetNodes         func() []NodeStatus
	UpdateNodeConfig func(peerID uint16, key, applicationType, value string) error
}

type Server struct {
	addr             string
	adminUsername    string
	adminPassword    string
	getNodes         func() []NodeStatus
	updateNodeConfig func(peerID uint16, key, applicationType, value string) error
	uiHandler        http.Handler
	sessionsMu       sync.RWMutex
	sessions         map[string]time.Time
}

type configUpdateRequest struct {
	PeerID uint16 `json:"peerId"`
	Key    string `json:"key"`
	Type   string `json:"type"`
	Value  string `json:"value"`
}

const sessionCookieName = "clearlink_admin_session"

func NewServer(options Options) (*Server, error) {
	if options.GetNodes == nil {
		return nil, fmt.Errorf("GetNodes callback is required")
	}
	if options.UpdateNodeConfig == nil {
		return nil, fmt.Errorf("UpdateNodeConfig callback is required")
	}
	addr := options.Addr
	if addr == "" {
		addr = "0.0.0.0:44325"
	}
	return &Server{
		addr:             addr,
		adminUsername:    options.AdminUsername,
		adminPassword:    options.AdminPassword,
		getNodes:         options.GetNodes,
		updateNodeConfig: options.UpdateNodeConfig,
		sessions:         make(map[string]time.Time),
	}, nil
}

func (s *Server) Start() {
	s.uiHandler = newUIHandler(s.getNodes)

	mux := http.NewServeMux()
	mux.HandleFunc("/app.js", handleNoopAppJS)
	mux.HandleFunc("/wasm_exec.js", handleNoopWasmExec)
	mux.HandleFunc("/app-worker.js", handleNoopAppWorker)
	mux.HandleFunc("/manifest.webmanifest", handleManifest)
	mux.HandleFunc("/app.css", handleAppCSS)
	mux.HandleFunc("/api/public/topology", s.handlePublicTopology)
	mux.HandleFunc("/api/public/connections", s.handlePublicConnections)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/admin/config", s.handleAdminConfigFormUpdate)
	mux.HandleFunc("/api/admin/nodes", s.handleAdminNodes)
	mux.HandleFunc("/api/admin/nodes/config", s.handleAdminNodeConfigUpdate)
	mux.Handle("/", http.HandlerFunc(s.handleUI))

	go func() {
		log.Printf("Web admin panel running at http://%s", s.addr)
		if err := http.ListenAndServe(s.addr, mux); err != nil {
			log.Printf("Web admin panel stopped: %v", err)
		}
	}()
}

func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if strings.HasPrefix(r.URL.Path, "/admin") && !s.isAuthorized(r) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if r.URL.Path == "/login" && s.isAuthorized(r) {
		http.Redirect(w, r, "/admin", http.StatusFound)
		return
	}

	s.uiHandler.ServeHTTP(w, r)
}

func (s *Server) handlePublicTopology(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	topology := buildPublicTopology(s.getNodes())
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(topology); err != nil {
		log.Printf("Failed to encode topology: %v", err)
	}
}

func (s *Server) handlePublicConnections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	response := buildPublicConnections(s.getNodes())
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode connections: %v", err)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.handleUI(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	if username != s.adminUsername || password != s.adminPassword {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	token, err := generateSessionToken()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	s.sessionsMu.Lock()
	s.sessions[token] = time.Now()
	s.sessionsMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		s.sessionsMu.Lock()
		delete(s.sessions, cookie.Value)
		s.sessionsMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) handleAdminNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.isAuthorized(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.getNodes()); err != nil {
		log.Printf("Failed to encode nodes: %v", err)
	}
}

func (s *Server) handleAdminNodeConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.isAuthorized(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	defer r.Body.Close()

	var payload configUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if payload.PeerID == 0 || payload.Key == "" || payload.Type == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := s.updateNodeConfig(payload.PeerID, payload.Key, payload.Type, payload.Value); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminConfigFormUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.isAuthorized(r) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	peerIDValue := strings.TrimSpace(r.FormValue("peerId"))
	key := strings.TrimSpace(r.FormValue("key"))
	appType := strings.TrimSpace(r.FormValue("type"))
	value := r.FormValue("value")

	peerID64, err := strconv.ParseUint(peerIDValue, 10, 16)
	if err != nil || peerID64 == 0 || key == "" || appType == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := s.updateNodeConfig(uint16(peerID64), key, appType, value); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (s *Server) isAuthorized(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	s.sessionsMu.RLock()
	_, ok := s.sessions[cookie.Value]
	s.sessionsMu.RUnlock()
	return ok
}

func generateSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func buildPublicTopology(nodes []NodeStatus) publicTopology {
	topology := publicTopology{
		Listen:    make([]publicNode, 0),
		Server:    publicNode{Name: "Server", RSSI: nil},
		Broadcast: make([]publicNode, 0),
	}

	for _, node := range nodes {
		name := strings.TrimSpace(node.Name)
		if name == "" {
			name = fmt.Sprintf("Peer %d", node.PeerID)
		}
		public := publicNode{PeerID: node.PeerID, Name: name, Active: node.Active, RSSI: node.RSSI}
		switch node.NodeType {
		case "listen":
			topology.Listen = append(topology.Listen, public)
		case "broadcast":
			topology.Broadcast = append(topology.Broadcast, public)
		}
	}

	return topology
}

func buildPublicConnections(nodes []NodeStatus) publicConnectionsResponse {
	response := publicConnectionsResponse{Connections: make([]publicConnection, 0, len(nodes))}

	var activeListenPeerID uint16
	for _, node := range nodes {
		if node.NodeType == "listen" && node.Active {
			activeListenPeerID = node.PeerID
			break
		}
	}

	for _, node := range nodes {
		if node.NodeType != "listen" && node.NodeType != "broadcast" {
			continue
		}
		color := "#6b7280"
		if activeListenPeerID != 0 {
			if node.NodeType == "listen" && node.PeerID == activeListenPeerID {
				color = "#3b82f6"
			}
			if node.NodeType == "broadcast" {
				color = "#3b82f6"
			}
		}

		response.Connections = append(response.Connections, publicConnection{
			ID:           fmt.Sprintf("peer-%d-to-server", node.PeerID),
			FromNodeID:   fmt.Sprintf("peer-%d", node.PeerID),
			ToNodeID:     "server-node",
			Color:        color,
			Width:        2,
			Opacity:      0.85,
			Curvature:    0.55,
			StartOffsetY: 30,
			EndOffsetY:   30,
			DashArray:    "",
		})
	}

	return response
}

func handleNoopAppJS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	_, _ = w.Write([]byte("// Server-rendered mode: no client wasm runtime is required.\n"))
}

func handleNoopWasmExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	_, _ = w.Write([]byte("// wasm_exec.js is intentionally not used in server-rendered mode.\n"))
}

func handleNoopAppWorker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript")
	_, _ = w.Write([]byte("// Service worker disabled for server-rendered mode.\n"))
}

func handleManifest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json")
	_, _ = w.Write([]byte(`{"name":"ClearLink","short_name":"ClearLink","start_url":"/","display":"standalone"}`))
}

func handleAppCSS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/css")
	_, _ = w.Write([]byte(`#app-wasm-loader{display:none!important;}`))
}
