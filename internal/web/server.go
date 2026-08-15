package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/api/public/topology", s.handlePublicTopology)
	mux.HandleFunc("/api/public/connections", s.handlePublicConnections)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/admin", s.handleAdmin)
	mux.HandleFunc("/api/admin/nodes", s.handleAdminNodes)
	mux.HandleFunc("/api/admin/nodes/config", s.handleAdminNodeConfigUpdate)

	go func() {
		log.Printf("Web admin panel running at http://%s", s.addr)
		if err := http.ListenAndServe(s.addr, mux); err != nil {
			log.Printf("Web admin panel stopped: %v", err)
		}
	}()
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(homeHTML))
}

func (s *Server) handlePublicTopology(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	nodes := s.getNodes()
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

	nodes := s.getNodes()
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

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode connections: %v", err)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if s.isAuthorized(r) {
			http.Redirect(w, r, "/admin", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(strings.Replace(loginHTML, "{{ERROR}}", "", 1)))
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
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(strings.Replace(loginHTML, "{{ERROR}}", "Invalid username or password.", 1)))
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

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.isAuthorized(r) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(adminHTML))
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
	fmt.Printf("Received config update request: %s %s\n", r.Method, r.URL.Path)
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

const loginHTML = `<!doctype html>
<html lang="en">
<head>
	<meta charset="utf-8" />
	<meta name="viewport" content="width=device-width, initial-scale=1" />
	<title>ClearLink Admin Login</title>
	<style>
		:root { color-scheme: dark; }
		body { font-family: Arial, sans-serif; margin: 0; min-height: 100vh; display: grid; place-items: center; background: #0b1220; color: #e5e7eb; }
		.card { width: min(420px, 92vw); background: #111827; border: 1px solid #1f2937; border-radius: 12px; padding: 24px; }
		h1 { margin: 0 0 16px 0; font-size: 1.4rem; }
		label { display: block; margin-top: 10px; margin-bottom: 6px; color: #9ca3af; }
		input { width: 100%; box-sizing: border-box; background: #0f172a; color: #e5e7eb; border: 1px solid #334155; border-radius: 8px; padding: 10px; }
		button { margin-top: 14px; width: 100%; border: 0; border-radius: 8px; padding: 10px; background: #2563eb; color: white; font-weight: 600; cursor: pointer; }
		.error { color: #fca5a5; min-height: 20px; margin-top: 8px; }
	</style>
</head>
<body>
	<form class="card" method="post" action="/login">
		<h1>System Admin Login</h1>
		<label for="username">Username</label>
		<input id="username" name="username" type="text" required />
		<label for="password">Password</label>
		<input id="password" name="password" type="password" required />
		<div class="error">{{ERROR}}</div>
		<button type="submit">Login</button>
	</form>
</body>
</html>`

const homeHTML = `<!doctype html>
<html lang="en">
<head>
	<meta charset="utf-8" />
	<meta name="viewport" content="width=device-width, initial-scale=1" />
	<title>ClearLink</title>
	<style>
		:root { color-scheme: dark; }
		body { font-family: Arial, sans-serif; margin: 24px; background: #0b1220; color: #e5e7eb; }
		.top { display: flex; justify-content: flex-end; margin-bottom: 18px; }
		.admin-btn { border: 0; border-radius: 8px; padding: 8px 12px; background: #2563eb; color: white; font-weight: 600; text-decoration: none; }
		h1 { text-align: center; margin: 0 0 20px 0; font-size: 2rem; }
		.graph-wrap { position: relative; }
		.graph { display: grid; gap: 20px; position: relative; z-index: 2; }
		.connections { position: absolute; inset: 0; width: 100%; height: 100%; pointer-events: none; z-index: 1; overflow: visible; }
		.row-title { color: #9ca3af; font-size: 0.9rem; margin-bottom: 8px; text-transform: uppercase; letter-spacing: 0.08em; }
		.row { display: flex; gap: 10px; flex-wrap: wrap; align-items: center; justify-content: center; }
		.node { background: #111827; border: 1px solid #1f2937; border-radius: 10px; min-width: 160px; padding: 10px 12px; }
		.node.active { background: #172554; border-color: #2563eb; }
		.node-name { font-weight: 700; }
		.node-rssi { color: #93c5fd; margin-top: 4px; }
		.server-node { min-width: 200px; text-align: center; }
		.empty { color: #6b7280; font-style: italic; }
	</style>
</head>
<body>
	<div class="top">
		<a class="admin-btn" href="/admin">Admin Panel</a>
	</div>
	<h1>ClearLink</h1>
	<div class="graph-wrap" id="graph-wrap">
		<svg id="connections" class="connections" aria-hidden="true"></svg>
		<div class="graph" id="graph">
			<div>
				<div class="row-title">Listen Nodes</div>
				<div class="row" id="listen-row"></div>
			</div>
			<div>
				<div class="row-title">Server</div>
				<div class="row" id="server-row"></div>
			</div>
			<div>
				<div class="row-title">Broadcast Nodes</div>
				<div class="row" id="broadcast-row"></div>
			</div>
		</div>
	</div>

	<script>
		function esc(text) {
			return String(text)
				.replaceAll('&', '&amp;')
				.replaceAll('<', '&lt;')
				.replaceAll('>', '&gt;')
				.replaceAll('"', '&quot;')
				.replaceAll("'", '&#39;');
		}

		function nodeHTML(node, showRSSI) {
			const rssi = node.rssi === null ? 'N/A' : node.rssi.toFixed(2) + ' dB';
			const activeClass = node.active ? ' active' : '';
			const rssiHTML = showRSSI ? ('<div class="node-rssi">RSSI: ' + rssi + '</div>') : '';
			return '<div class="node graph-node' + activeClass + '" data-node-id="peer-' + node.peerId + '"><div class="node-name">' + esc(node.name) + '</div>' + rssiHTML + '</div>';
		}

		function serverNodeHTML() {
			return '<div class="node server-node graph-node" id="server-node"><div class="node-name">Server</div></div>';
		}

		function renderRow(elementId, nodes, showRSSI) {
			const root = document.getElementById(elementId);
			if (!nodes || !nodes.length) {
				root.innerHTML = '<div class="empty">No nodes</div>';
				return;
			}
			root.innerHTML = nodes.map((node) => nodeHTML(node, showRSSI)).join('');
		}

		function drawConnections(connections) {
			const wrap = document.getElementById('graph-wrap');
			const svg = document.getElementById('connections');
			if (!wrap || !svg || !connections) {
				return;
			}

			const wrapRect = wrap.getBoundingClientRect();
			svg.setAttribute('viewBox', '0 0 ' + Math.max(1, Math.round(wrapRect.width)) + ' ' + Math.max(1, Math.round(wrapRect.height)));
			svg.innerHTML = '';

			connections.forEach((connection) => {
				const fromNode = document.querySelector('[data-node-id="' + connection.fromNodeId + '"]');
				const toNode = connection.toNodeId === 'server-node'
					? document.getElementById('server-node')
					: document.querySelector('[data-node-id="' + connection.toNodeId + '"]');
				if (!fromNode || !toNode) {
					return;
				}

				const fromRect = fromNode.getBoundingClientRect();
				const toRect = toNode.getBoundingClientRect();
				const startX = (fromRect.left + fromRect.width / 2) - wrapRect.left;
				const startY = (fromRect.top + fromRect.height / 2) - wrapRect.top;
				const endX = (toRect.left + toRect.width / 2) - wrapRect.left;
				const endY = (toRect.top + toRect.height / 2) - wrapRect.top;

				const curvature = typeof connection.curvature === 'number' ? connection.curvature : 0.55;
				const startOffset = typeof connection.startOffsetY === 'number' ? connection.startOffsetY : 30;
				const endOffset = typeof connection.endOffsetY === 'number' ? connection.endOffsetY : 30;
				const ctrlOffsetY = Math.abs(endY - startY) * curvature;
				const startCtrlY = startY + (startY < endY ? (ctrlOffsetY + startOffset) : -(ctrlOffsetY + startOffset));
				const endCtrlY = endY + (startY < endY ? -(ctrlOffsetY + endOffset) : (ctrlOffsetY + endOffset));

				const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
				path.setAttribute('d', 'M ' + startX + ' ' + startY + ' C ' + startX + ' ' + startCtrlY + ', ' + endX + ' ' + endCtrlY + ', ' + endX + ' ' + endY);
				path.setAttribute('stroke', connection.color || '#ef4444');
				path.setAttribute('stroke-width', String(connection.width || 2));
				path.setAttribute('fill', 'none');
				path.setAttribute('opacity', String(connection.opacity || 0.85));
				if (connection.dashArray) {
					path.setAttribute('stroke-dasharray', connection.dashArray);
				}
				svg.appendChild(path);
			});
		}

		async function refreshGraph() {
			const topologyResponse = await fetch('/api/public/topology', { cache: 'no-store' });
			const connectionsResponse = await fetch('/api/public/connections', { cache: 'no-store' });
			const data = await topologyResponse.json();
			const connectionsData = await connectionsResponse.json();
			renderRow('listen-row', data.listen || [], true);
			document.getElementById('server-row').innerHTML = serverNodeHTML();
			renderRow('broadcast-row', data.broadcast || [], false);
			requestAnimationFrame(() => drawConnections((connectionsData && connectionsData.connections) || []));
		}

		window.addEventListener('resize', () => refreshGraph());
		refreshGraph();
		setInterval(refreshGraph, 150);
	</script>
</body>
</html>`

const adminHTML = `<!doctype html>
<html lang="en">
<head>
	<meta charset="utf-8" />
	<meta name="viewport" content="width=device-width, initial-scale=1" />
	<title>ClearLink Admin Panel</title>
	<style>
		:root { color-scheme: dark; }
		body { font-family: Arial, sans-serif; margin: 24px; background: #0b1220; color: #e5e7eb; }
		h1 { margin: 0; }
		.top { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
		.top-actions { display: flex; gap: 8px; align-items: center; }
		button { border: 0; border-radius: 8px; padding: 8px 12px; background: #2563eb; color: white; font-weight: 600; cursor: pointer; }
		button.secondary { background: #334155; }
		.top-actions a { border-radius: 8px; padding: 8px 12px; background: #334155; color: white; font-weight: 600; text-decoration: none; display: inline-block; }
		.card { background: #111827; border: 1px solid #1f2937; border-radius: 12px; padding: 14px; margin-bottom: 14px; }
		.grid { display: grid; gap: 8px; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); margin: 10px 0; }
		.meta { color: #9ca3af; font-size: 0.92rem; }
		table { width: 100%; border-collapse: collapse; }
		th, td { border: 1px solid #1f2937; padding: 8px; text-align: left; }
		th { background: #0f172a; }
		input { width: 100%; box-sizing: border-box; background: #0f172a; color: #e5e7eb; border: 1px solid #334155; border-radius: 8px; padding: 7px; }
		.status { margin-top: 10px; color: #93c5fd; min-height: 18px; }
	</style>
</head>
<body>
	<div class="top">
		<h1>Admin Panel</h1>
		<div class="top-actions">
			<a href="/">Home</a>
			<form method="post" action="/logout"><button class="secondary" type="submit">Logout</button></form>
		</div>
	</div>
	<div id="nodes"></div>
	<div class="status" id="status"></div>
	<script>
		const statusEl = document.getElementById('status');
		const draftValues = {};
		function esc(text) {
			return String(text)
				.replaceAll('&', '&amp;')
				.replaceAll('<', '&lt;')
				.replaceAll('>', '&gt;')
				.replaceAll('"', '&quot;')
				.replaceAll("'", '&#39;');
		}
		async function refreshNodes() {
			const active = document.activeElement;
			if (active && active.classList && active.classList.contains('cfg-input')) {
				return;
			}
			document.querySelectorAll('.cfg-input').forEach((input) => {
				draftValues[input.id] = input.value;
			});
			const response = await fetch('/api/admin/nodes', { cache: 'no-store' });
			if (response.status === 401) {
				window.location.href = '/login';
				return;
			}
			const nodes = await response.json();
			const root = document.getElementById('nodes');
			if (!nodes.length) {
				root.innerHTML = '<div class="card">No connected nodes.</div>';
				return;
			}
			root.innerHTML = nodes.map((node) => {
				const rssiText = node.nodeType === 'listen'
					? ('RSSI: ' + (node.rssi === null ? 'N/A' : node.rssi.toFixed(2) + ' dB'))
					: 'RSSI: N/A';
				const configRows = (node.config || []).map((entry) => {
					const fieldId = 'cfg-' + node.peerId + '-' + entry.type + '-' + entry.key;
					const hasDraft = Object.prototype.hasOwnProperty.call(draftValues, fieldId);
					const fieldValue = hasDraft ? draftValues[fieldId] : entry.value;
					return '<tr>' +
						'<td>' + esc(entry.type) + '</td>' +
						'<td>' + esc(entry.key) + '</td>' +
						'<td><input class="cfg-input" id="' + esc(fieldId) + '" value="' + esc(fieldValue) + '" /></td>' +
						'<td><button onclick="saveConfig(' + node.peerId + ', \'' + esc(entry.type) + '\', \'' + esc(entry.key) + '\', \'' + esc(fieldId) + '\')">Save</button></td>' +
						'</tr>';
				}).join('');
				return '<div class="card">' +
					'<div><strong>Node ' + node.peerId + '</strong> ' + esc(node.name) + '</div>' +
					'<div class="grid meta">' +
						'<div>Address: ' + esc(node.remoteAddr) + '</div>' +
						'<div>Last heartbeat: ' + esc(node.lastHeartbeatAgo) + ' (' + esc(node.lastHeartbeat) + ')</div>' +
						'<div>' + rssiText + '</div>' +
					'</div>' +
					'<table>' +
						'<thead><tr><th>Type</th><th>Key</th><th>Value</th><th>Action</th></tr></thead>' +
						'<tbody>' + configRows + '</tbody>' +
					'</table>' +
				'</div>';
			}).join('');
		}
		async function saveConfig(peerId, type, key, fieldId) {
			const input = document.getElementById(fieldId);
			draftValues[fieldId] = input.value;
			const body = { peerId, type, key, value: input.value };
			const response = await fetch('/api/admin/nodes/config', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(body),
			});
			if (response.status === 204) {
				delete draftValues[fieldId];
				statusEl.textContent = 'Saved ' + type + ':' + key + ' for node ' + peerId;
				setTimeout(refreshNodes, 250);
				return;
			}
			const err = await response.text();
			statusEl.textContent = err || 'Failed to save config value.';
		}
		refreshNodes();
		setInterval(refreshNodes, 2500);
	</script>
</body>
</html>`
