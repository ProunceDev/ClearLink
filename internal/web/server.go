package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

const (
	adminSessionCookieName = "clearlink_admin_session"
	adminSessionLifetime   = 30 * 24 * time.Hour
	maxMediaUploadSize     = 100 * 1024 * 1024
)

type adminSessionPayload struct {
	Username  string `json:"username"`
	ExpiresAt int64  `json:"expiresAt"`
	Nonce     string `json:"nonce"`
}

type NodeConfig struct {
	Key       string `json:"key"`
	Type      string `json:"type"`
	Value     string `json:"value"`
	ValueType string `json:"valueType"`
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
	SNR              *float64     `json:"snr"`
	LastKeyUpAgo     string       `json:"lastKeyUpAgo"`
	Config           []NodeConfig `json:"config"`
}

type publicNode struct {
	PeerID       uint16   `json:"peerId"`
	Name         string   `json:"name"`
	Active       bool     `json:"active"`
	RSSI         *float64 `json:"rssi"`
	SNR          *float64 `json:"snr"`
	LastKeyUpAgo string   `json:"lastKeyUpAgo"`
}

type publicTopology struct {
	Listen    []publicNode `json:"listen"`
	Interface []publicNode `json:"interface"`
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

type BulkSaveConfigRequest struct {
	NodeType string `json:"nodeType"`
	Key      string `json:"key"`
	Value    string `json:"value"`
}

type BulkSaveConfigResult struct {
	PeerID  uint16 `json:"peerId"`
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type BulkSaveConfigResponse struct {
	Attempted int                    `json:"attempted"`
	Updated   int                    `json:"updated"`
	Results   []BulkSaveConfigResult `json:"results"`
}

type MediaAsset struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	SizeBytes  int64  `json:"sizeBytes"`
	UploadedAt string `json:"uploadedAt"`
	StreamURL  string `json:"streamUrl"`
}

type AudioEvent struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Enabled         bool     `json:"enabled"`
	MediaIDs        []string `json:"mediaIds"`
	IntervalSeconds int      `json:"intervalSeconds"`
	ScheduleAt      string   `json:"scheduleAt"`
	SourcePeerID    uint16   `json:"sourcePeerId"`
	SourceNodeType  string   `json:"sourceNodeType"`
}

type mediaMetadata struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	UploadedAt time.Time `json:"uploadedAt"`
}

type Options struct {
	Addr             string
	AdminUsername    string
	AdminPassword    string
	MediaDir         string
	GetNodes         func() []NodeStatus
	SaveConfig       func(peerID uint16, key, applicationType, value string) error
	GetAudioEvents   func() []AudioEvent
	SaveAudioEvent   func(event AudioEvent) (AudioEvent, error)
	DeleteAudioEvent func(eventID string) error
}

type Server struct {
	addr             string
	username         string
	password         string
	mediaDir         string
	getNodes         func() []NodeStatus
	saveConfig       func(peerID uint16, key, applicationType, value string) error
	getAudioEvents   func() []AudioEvent
	saveAudioEvent   func(event AudioEvent) (AudioEvent, error)
	deleteAudioEvent func(eventID string) error
}

func NewServer(options Options) (*Server, error) {
	if options.GetNodes == nil {
		return nil, fmt.Errorf("GetNodes callback is required")
	}
	addr := options.Addr
	if addr == "" {
		addr = "0.0.0.0:8080"
	}
	mediaDir := strings.TrimSpace(options.MediaDir)
	if mediaDir == "" {
		mediaDir = "media"
	}
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		return nil, fmt.Errorf("create media directory: %w", err)
	}
	return &Server{
		addr:             addr,
		username:         options.AdminUsername,
		password:         options.AdminPassword,
		mediaDir:         mediaDir,
		getNodes:         options.GetNodes,
		saveConfig:       options.SaveConfig,
		getAudioEvents:   options.GetAudioEvents,
		saveAudioEvent:   options.SaveAudioEvent,
		deleteAudioEvent: options.DeleteAudioEvent,
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
	mux.HandleFunc("/api/admin/logout", s.handleAdminLogout)
	mux.HandleFunc("/api/admin/nodes", s.handleAdminNodes)
	mux.HandleFunc("/api/admin/config", s.handleAdminConfig)
	mux.HandleFunc("/api/admin/config/bulk", s.handleAdminBulkConfig)
	mux.HandleFunc("/api/admin/media", s.handleAdminMedia)
	mux.HandleFunc("/api/admin/media/", s.handleAdminMediaItem)
	mux.HandleFunc("/api/admin/audio-events", s.handleAdminAudioEvents)
	mux.HandleFunc("/api/admin/audio-events/", s.handleAdminAudioEventItem)
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
	if s.hasValidAdminSession(r) {
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
		if err := s.setAdminSession(w, r); err != nil {
			log.Printf("Failed to create admin session: %v", err)
			http.Error(w, "unable to create admin session", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		return
	}

	http.Error(w, "invalid credentials", http.StatusUnauthorized)
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	s.clearAdminSession(w, r)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func (s *Server) setAdminSession(w http.ResponseWriter, r *http.Request) error {
	expiresAt := time.Now().Add(adminSessionLifetime)
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}

	payload, err := json.Marshal(adminSessionPayload{
		Username:  s.username,
		ExpiresAt: expiresAt.Unix(),
		Nonce:     base64.RawURLEncoding.EncodeToString(nonce),
	})
	if err != nil {
		return err
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(s.username+"\x00"+s.password))
	_, _ = mac.Write([]byte(encodedPayload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    encodedPayload + "." + signature,
		Path:     "/api/admin",
		Expires:  expiresAt,
		MaxAge:   int(adminSessionLifetime.Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (s *Server) clearAdminSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/api/admin",
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) hasValidAdminSession(r *http.Request) bool {
	cookie, err := r.Cookie(adminSessionCookieName)
	if err != nil {
		return false
	}

	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}

	providedSignature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(s.username+"\x00"+s.password))
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(providedSignature, mac.Sum(nil)) {
		return false
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	var payload adminSessionPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return false
	}

	return payload.Username == s.username && time.Now().Unix() < payload.ExpiresAt
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

func (s *Server) handleAdminBulkConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.isAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.saveConfig == nil {
		http.Error(w, "config save handler not configured", http.StatusNotImplemented)
		return
	}

	var req BulkSaveConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid bulk config payload", http.StatusBadRequest)
		return
	}
	req.Key = strings.TrimSpace(req.Key)
	req.NodeType = strings.ToLower(strings.TrimSpace(req.NodeType))
	if req.NodeType == "" {
		req.NodeType = "all"
	}
	if req.Key == "" {
		http.Error(w, "config key is required", http.StatusBadRequest)
		return
	}
	if req.NodeType != "all" && req.NodeType != "listen" && req.NodeType != "interface" && req.NodeType != "broadcast" {
		http.Error(w, "invalid node type", http.StatusBadRequest)
		return
	}

	response := BulkSaveConfigResponse{Results: make([]BulkSaveConfigResult, 0)}
	for _, node := range s.getNodes() {
		if req.NodeType != "all" && node.NodeType != req.NodeType {
			continue
		}
		for _, entry := range node.Config {
			if entry.Key != req.Key {
				continue
			}

			response.Attempted++
			result := BulkSaveConfigResult{PeerID: node.PeerID, Name: node.Name}
			if err := s.saveConfig(node.PeerID, entry.Key, entry.Type, req.Value); err != nil {
				result.Error = err.Error()
			} else {
				result.Success = true
				response.Updated++
			}
			response.Results = append(response.Results, result)
			break
		}
	}

	if response.Attempted == 0 {
		http.Error(w, "no matching connected-node settings found", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) handleAdminAudioEvents(w http.ResponseWriter, r *http.Request) {
	if !s.isAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.getAudioEvents == nil {
		http.Error(w, "audio events are not configured", http.StatusNotImplemented)
		return
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.getAudioEvents())
	case http.MethodPost:
		if s.saveAudioEvent == nil {
			http.Error(w, "audio event save handler is not configured", http.StatusNotImplemented)
			return
		}
		var event AudioEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			http.Error(w, "invalid audio event payload", http.StatusBadRequest)
			return
		}
		savedEvent, err := s.saveAudioEvent(event)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(savedEvent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAdminAudioEventItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.isAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.deleteAudioEvent == nil {
		http.Error(w, "audio event delete handler is not configured", http.StatusNotImplemented)
		return
	}

	eventID := strings.TrimPrefix(r.URL.Path, "/api/admin/audio-events/")
	if !isValidMediaID(eventID) {
		http.NotFound(w, r)
		return
	}
	if err := s.deleteAudioEvent(eventID); err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminMedia(w http.ResponseWriter, r *http.Request) {
	if !s.isAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		assets, err := s.listMediaAssets()
		if err != nil {
			log.Printf("Failed to list media assets: %v", err)
			http.Error(w, "unable to list media assets", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(assets)
	case http.MethodPost:
		s.handleMediaUpload(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAdminMediaItem(w http.ResponseWriter, r *http.Request) {
	if !s.isAuthorized(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/admin/media/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || !isValidMediaID(parts[0]) {
		http.NotFound(w, r)
		return
	}

	switch {
	case len(parts) == 1 && r.Method == http.MethodDelete:
		s.handleMediaDelete(w, r, parts[0])
	case len(parts) == 2 && parts[1] == "stream" && r.Method == http.MethodGet:
		s.handleMediaStream(w, r, parts[0])
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMediaUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxMediaUploadSize)
	if err := r.ParseMultipartForm(maxMediaUploadSize); err != nil {
		http.Error(w, "invalid or oversized media upload", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "MP3 file field is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	name := filepath.Base(strings.TrimSpace(header.Filename))
	if name == "." || name == "" || !strings.EqualFold(filepath.Ext(name), ".mp3") {
		http.Error(w, "only .mp3 files can be uploaded", http.StatusBadRequest)
		return
	}

	probe := make([]byte, 3)
	probeSize, readErr := io.ReadFull(file, probe)
	if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
		http.Error(w, "unable to read uploaded media", http.StatusBadRequest)
		return
	}
	if !hasMP3Signature(probe[:probeSize]) {
		http.Error(w, "uploaded file is not a valid MP3 stream", http.StatusBadRequest)
		return
	}

	id, err := s.newMediaID()
	if err != nil {
		log.Printf("Failed to create media ID: %v", err)
		http.Error(w, "unable to store media asset", http.StatusInternalServerError)
		return
	}

	temporaryFile, err := os.CreateTemp(s.mediaDir, ".upload-*")
	if err != nil {
		log.Printf("Failed to create media upload file: %v", err)
		http.Error(w, "unable to store media asset", http.StatusInternalServerError)
		return
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)
	defer temporaryFile.Close()

	if _, err := temporaryFile.Write(probe[:probeSize]); err != nil {
		http.Error(w, "unable to store media asset", http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(temporaryFile, file); err != nil {
		http.Error(w, "unable to store media asset", http.StatusInternalServerError)
		return
	}
	if err := temporaryFile.Close(); err != nil {
		http.Error(w, "unable to store media asset", http.StatusInternalServerError)
		return
	}

	mediaPath := s.mediaPath(id)
	if err := os.Rename(temporaryPath, mediaPath); err != nil {
		log.Printf("Failed to finalize media upload: %v", err)
		http.Error(w, "unable to store media asset", http.StatusInternalServerError)
		return
	}

	metadata := mediaMetadata{ID: id, Name: name, UploadedAt: time.Now().UTC()}
	if err := s.writeMediaMetadata(metadata); err != nil {
		_ = os.Remove(mediaPath)
		log.Printf("Failed to save media metadata: %v", err)
		http.Error(w, "unable to store media asset", http.StatusInternalServerError)
		return
	}

	asset, err := s.mediaAsset(metadata)
	if err != nil {
		log.Printf("Failed to read uploaded media: %v", err)
		http.Error(w, "unable to store media asset", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(asset)
}

func (s *Server) handleMediaDelete(w http.ResponseWriter, _ *http.Request, id string) {
	if _, err := os.Stat(s.mediaPath(id)); err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, nil)
			return
		}
		http.Error(w, "unable to access media asset", http.StatusInternalServerError)
		return
	}
	if err := os.Remove(s.mediaPath(id)); err != nil {
		http.Error(w, "unable to delete media asset", http.StatusInternalServerError)
		return
	}
	if err := os.Remove(s.mediaMetadataPath(id)); err != nil && !os.IsNotExist(err) {
		log.Printf("Failed to delete media metadata for %s: %v", id, err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMediaStream(w http.ResponseWriter, r *http.Request, id string) {
	asset, err := s.readMediaAsset(id)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "unable to read media asset", http.StatusInternalServerError)
		return
	}
	file, err := os.Open(s.mediaPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "unable to read media asset", http.StatusInternalServerError)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		http.Error(w, "unable to read media asset", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": asset.Name}))
	http.ServeContent(w, r, asset.Name, info.ModTime(), file)
}

func (s *Server) newMediaID() (string, error) {
	for range 5 {
		bytes := make([]byte, 16)
		if _, err := rand.Read(bytes); err != nil {
			return "", err
		}
		id := hex.EncodeToString(bytes)
		if _, err := os.Stat(s.mediaPath(id)); os.IsNotExist(err) {
			return id, nil
		} else if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("unable to allocate unique media ID")
}

func (s *Server) listMediaAssets() ([]MediaAsset, error) {
	entries, err := os.ReadDir(s.mediaDir)
	if err != nil {
		return nil, err
	}

	assets := make([]MediaAsset, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !isValidMediaID(id) {
			continue
		}
		asset, err := s.readMediaAsset(id)
		if err != nil {
			if !os.IsNotExist(err) {
				log.Printf("Skipping unreadable media asset %s: %v", id, err)
			}
			continue
		}
		assets = append(assets, asset)
	}

	sort.Slice(assets, func(i, j int) bool {
		return assets[i].UploadedAt > assets[j].UploadedAt
	})
	return assets, nil
}

func (s *Server) readMediaAsset(id string) (MediaAsset, error) {
	contents, err := os.ReadFile(s.mediaMetadataPath(id))
	if err != nil {
		return MediaAsset{}, err
	}
	var metadata mediaMetadata
	if err := json.Unmarshal(contents, &metadata); err != nil {
		return MediaAsset{}, err
	}
	if metadata.ID != id || metadata.Name == "" {
		return MediaAsset{}, fmt.Errorf("invalid media metadata")
	}
	return s.mediaAsset(metadata)
}

func (s *Server) mediaAsset(metadata mediaMetadata) (MediaAsset, error) {
	info, err := os.Stat(s.mediaPath(metadata.ID))
	if err != nil {
		return MediaAsset{}, err
	}
	return MediaAsset{
		ID:         metadata.ID,
		Name:       metadata.Name,
		SizeBytes:  info.Size(),
		UploadedAt: metadata.UploadedAt.UTC().Format(time.RFC3339),
		StreamURL:  "/api/admin/media/" + metadata.ID + "/stream",
	}, nil
}

func (s *Server) writeMediaMetadata(metadata mediaMetadata) error {
	contents, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	temporaryFile, err := os.CreateTemp(s.mediaDir, ".metadata-*")
	if err != nil {
		return err
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporaryFile.Write(contents); err != nil {
		_ = temporaryFile.Close()
		return err
	}
	if err := temporaryFile.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, s.mediaMetadataPath(metadata.ID))
}

func (s *Server) mediaPath(id string) string {
	return filepath.Join(s.mediaDir, id+".mp3")
}

func (s *Server) mediaMetadataPath(id string) string {
	return filepath.Join(s.mediaDir, id+".json")
}

func isValidMediaID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, char := range id {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func hasMP3Signature(contents []byte) bool {
	if len(contents) >= 3 && string(contents[:3]) == "ID3" {
		return true
	}
	return len(contents) >= 2 && contents[0] == 0xff && contents[1]&0xe0 == 0xe0
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
		Interface: make([]publicNode, 0),
		Server:    publicNode{Name: "Server", RSSI: nil},
		Broadcast: make([]publicNode, 0),
	}

	for _, node := range nodes {
		name := strings.TrimSpace(node.Name)
		if name == "" {
			name = fmt.Sprintf("Peer %d", node.PeerID)
		}
		public := publicNode{
			PeerID:       node.PeerID,
			Name:         name,
			Active:       node.Active,
			RSSI:         node.RSSI,
			SNR:          node.SNR,
			LastKeyUpAgo: node.LastKeyUpAgo,
		}
		switch node.NodeType {
		case "listen":
			topology.Listen = append(topology.Listen, public)
		case "interface":
			topology.Interface = append(topology.Interface, public)
		case "broadcast":
			topology.Broadcast = append(topology.Broadcast, public)
		}
	}

	return topology
}

func buildPublicConnections(nodes []NodeStatus) publicConnectionsResponse {
	response := publicConnectionsResponse{Connections: make([]publicConnection, 0, len(nodes)*2)}

	for _, node := range nodes {
		if node.NodeType != "listen" && node.NodeType != "interface" {
			continue
		}
		color := "#6b7280"
		if node.Active {
			color = "#ef4444"
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
		if node.NodeType != "broadcast" && node.NodeType != "interface" {
			continue
		}
		color := "#6b7280"
		if node.Active {
			color = "#ef4444"
		}
		response.Connections = append(response.Connections, publicConnection{
			ID:           fmt.Sprintf("server-to-peer-%d", node.PeerID),
			FromNodeID:   "server-node",
			ToNodeID:     fmt.Sprintf("peer-%d", node.PeerID),
			Color:        color,
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
