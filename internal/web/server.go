package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
)

//go:embed static/*
var staticFiles embed.FS

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

type SaveConfigRequest struct {
	PeerID uint16 `json:"peerId"`
	Key    string `json:"key"`
	Type   string `json:"type"`
	Value  string `json:"value"`
}

type Options struct {
	Addr           string
	AdminUsername  string
	AdminPassword  string
	GetNodes       func() []NodeStatus
	SaveConfig     func(peerID uint16, key, applicationType, value string) error
}

type Server struct {
	addr       string
	username   string
	password   string
	getNodes   func() []NodeStatus
	saveConfig func(peerID uint16, key, applicationType, value string) error
}

func NewServer(options Options) (*Server, error) {
	if options.GetNodes == nil {
		return nil, fmt.Errorf("GetNodes callback is required")
	}
	addr := options.Addr
	if addr == "" {
		addr = "0.0.0.0:8080"
	}
	return &Server{
		addr:       addr,
		username:   options.AdminUsername,
		password:   options.AdminPassword,
		getNodes:   options.GetNodes,
		saveConfig: options.SaveConfig,
	}, nil
}

func (s *Server) Start() {
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("embedded web assets not found; run the flutter build first: %v", err)
	}

	fileServer := http.FileServer(http.FS(staticFS))

	mux := http.NewServeMux()

	mux.HandleFunc("/api/public/topology", s.handlePublicTopology)
	mux.HandleFunc("/api/public/connections", s.handlePublicConnections)
	mux.HandleFunc("/api/admin/login", s.handleAdminLogin)
	mux.HandleFunc("/api/admin/nodes", s.handleAdminNodes)
	mux.HandleFunc("/api/admin/config", s.handleAdminConfig)
	mux.HandleFunc("/api/health", s.handleHealth)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		if r.URL.Path == "/" || !strings.Contains(r.URL.Path, ".") {
			index, err := fs.ReadFile(staticFS, "index.html")
			if err != nil {
				http.Error(w, "index.html not found", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)

			if r.Method != http.MethodHead {
				_, _ = w.Write(index)
			}
			return
		}

		fileServer.ServeHTTP(w, r)
	})

	log.Printf("API server running at http://%s", s.addr)

	if err := http.ListenAndServe(s.addr, mux); err != nil {
		log.Printf("API server stopped: %v", err)
	}
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

func (s *Server) isAuthorized(r *http.Request) bool {
	if s.username == "" && s.password == "" {
		return true
	}
	username, password, ok := r.BasicAuth()
	return ok && username == s.username && password == s.password
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "invalid login payload", http.StatusBadRequest)
		return
	}

	if creds.Username == s.username && creds.Password == s.password {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		return
	}

	http.Error(w, "invalid credentials", http.StatusUnauthorized)
}

func (s *Server) handleAdminNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.isAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.getNodes()); err != nil {
		log.Printf("Failed to encode admin nodes: %v", err)
	}
}

func (s *Server) handleAdminConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.isAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(s.getNodes()); err != nil {
			log.Printf("Failed to encode config nodes: %v", err)
		}
		return
	}

	if s.saveConfig == nil {
		http.Error(w, "config save handler not configured", http.StatusNotImplemented)
		return
	}

	var req SaveConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid config payload", http.StatusBadRequest)
		return
	}

	if err := s.saveConfig(req.PeerID, req.Key, req.Type, req.Value); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
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

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
	response := publicConnectionsResponse{Connections: make([]publicConnection, 0, len(nodes)*2)}

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
				color = "#ef4444"
			}
			if node.NodeType == "broadcast" && node.Active {
				color = "#ef4444"
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

	for _, node := range nodes {
		if node.NodeType != "broadcast" || !node.Active {
			continue
		}
		response.Connections = append(response.Connections, publicConnection{
			ID:           fmt.Sprintf("server-to-peer-%d", node.PeerID),
			FromNodeID:   "server-node",
			ToNodeID:     fmt.Sprintf("peer-%d", node.PeerID),
			Color:        "#ef4444",
			Width:        2,
			Opacity:      0.85,
			Curvature:    0.55,
			StartOffsetY: -30,
			EndOffsetY:   -30,
			DashArray:    "",
		})
	}

	return response
}
