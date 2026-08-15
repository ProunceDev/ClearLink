package web

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

var (
	uiRoutesOnce sync.Once
	uiStateMu    sync.RWMutex
	uiState      uiProvider
)

type uiProvider struct {
	getNodes func() []NodeStatus
}

func newUIHandler(getNodes func() []NodeStatus) http.Handler {
	uiStateMu.Lock()
	uiState = uiProvider{getNodes: getNodes}
	uiStateMu.Unlock()

	uiRoutesOnce.Do(func() {
		app.Route("/", func() app.Composer { return &homePage{} })
		app.Route("/login", func() app.Composer { return &loginPage{} })
		app.Route("/admin", func() app.Composer { return &adminPage{} })
	})

	app.RunWhenOnBrowser()

	return &app.Handler{
		Name:        "ClearLink",
		Title:       "ClearLink",
		Description: "ClearLink control panel",
	}
}

func currentNodes() []NodeStatus {
	uiStateMu.RLock()
	getNodes := uiState.getNodes
	uiStateMu.RUnlock()
	if getNodes == nil {
		return nil
	}
	return getNodes()
}

type homePage struct {
	app.Compo
}

func (p *homePage) Render() app.UI {
	nodes := currentNodes()
	topology := buildPublicTopology(nodes)
	connections := buildPublicConnections(nodes)

	return app.Div().Body(
		panelStyles(),
		app.Div().Class("top").Body(
			app.A().Class("admin-btn").Href("/admin").Text("Admin Panel"),
		),
		app.H1().Text("ClearLink"),
		app.P().Class("subtle").Text("Reload the page for latest topology."),
		homeSection("Listen Nodes", topology.Listen, true),
		homeSection("Server", []publicNode{{Name: "Server"}}, false),
		homeSection("Broadcast Nodes", topology.Broadcast, false),
		app.Div().Class("card").Body(
			app.H2().Text("Connections"),
			renderConnectionList(connections.Connections),
		),
	)
}

func homeSection(title string, nodes []publicNode, showRSSI bool) app.UI {
	items := make([]app.UI, 0, len(nodes))
	if len(nodes) == 0 {
		items = append(items, app.Div().Class("empty").Text("No nodes"))
	} else {
		for _, node := range nodes {
			items = append(items, renderPublicNode(node, showRSSI))
		}
	}

	return app.Div().Class("card").Body(
		app.Div().Class("row-title").Text(title),
		app.Div().Class("row").Body(items...),
	)
}

func renderPublicNode(node publicNode, showRSSI bool) app.UI {
	card := app.Div().Class("node")
	if node.Active {
		card = card.Class("active")
	}

	children := []app.UI{app.Div().Class("node-name").Text(node.Name)}
	if showRSSI {
		rssiText := "RSSI: N/A"
		if node.RSSI != nil {
			rssiText = fmt.Sprintf("RSSI: %.2f dB", *node.RSSI)
		}
		children = append(children, app.Div().Class("node-rssi").Text(rssiText))
	}
	return card.Body(children...)
}

func renderConnectionList(connections []publicConnection) app.UI {
	if len(connections) == 0 {
		return app.Div().Class("empty").Text("No active links")
	}

	items := make([]app.UI, 0, len(connections))
	for _, c := range connections {
		state := "idle"
		if strings.EqualFold(c.Color, "#3b82f6") {
			state = "active"
		}
		items = append(items, app.Li().Text(fmt.Sprintf("%s -> %s (%s)", c.FromNodeID, c.ToNodeID, state)))
	}
	return app.Ul().Class("connections-list").Body(items...)
}

type loginPage struct {
	app.Compo
}

func (p *loginPage) Render() app.UI {
	return app.Div().Body(
		panelStyles(),
		app.Div().Class("login-wrap").Body(
			app.Form().Class("card login-card").Method(http.MethodPost).Action("/login").Body(
				app.H1().Text("System Admin Login"),
				app.P().Class("subtle").Text("Use the configured server admin credentials."),
				app.Label().For("username").Text("Username"),
				app.Input().ID("username").Name("username").Type("text").Required(true),
				app.Label().For("password").Text("Password"),
				app.Input().ID("password").Name("password").Type("password").Required(true),
				app.Button().Type("submit").Text("Login"),
			),
		),
	)
}

type adminPage struct {
	app.Compo
}

func (p *adminPage) Render() app.UI {
	nodes := currentNodes()
	cards := make([]app.UI, 0, len(nodes))
	for _, node := range nodes {
		cards = append(cards, renderAdminNode(node))
	}
	if len(cards) == 0 {
		cards = append(cards, app.Div().Class("card").Text("No connected nodes."))
	}

	return app.Div().Body(
		panelStyles(),
		app.Div().Class("top").Body(
			app.H1().Text("Admin Panel"),
			app.Div().Class("top-actions").Body(
				app.A().Href("/").Text("Home"),
				app.Form().Method(http.MethodPost).Action("/logout").Body(
					app.Button().Type("submit").Class("secondary").Text("Logout"),
				),
			),
		),
		app.P().Class("subtle").Text("Edits post directly to the server. Reload to get fresh status."),
		app.Div().Body(cards...),
	)
}

func renderAdminNode(node NodeStatus) app.UI {
	rssi := "RSSI: N/A"
	if node.NodeType == "listen" && node.RSSI != nil {
		rssi = fmt.Sprintf("RSSI: %.2f dB", *node.RSSI)
	}

	rows := make([]app.UI, 0, len(node.Config))
	for _, cfg := range node.Config {
		rows = append(rows, app.Tr().Body(
			app.Td().Text(cfg.Type),
			app.Td().Text(cfg.Key),
			app.Td().Body(
				app.Form().Class("cfg-form").Method(http.MethodPost).Action("/admin/config").Body(
					app.Input().Type("hidden").Name("peerId").Value(fmt.Sprintf("%d", node.PeerID)),
					app.Input().Type("hidden").Name("type").Value(cfg.Type),
					app.Input().Type("hidden").Name("key").Value(cfg.Key),
					app.Input().Type("text").Name("value").Value(cfg.Value).Required(true),
					app.Button().Type("submit").Text("Save"),
				),
			),
		))
	}

	if len(rows) == 0 {
		rows = append(rows, app.Tr().Body(
			app.Td().ColSpan(3).Class("empty").Text("No editable config entries"),
		))
	}

	header := app.THead().Body(app.Tr().Body(
		app.Th().Text("Type"),
		app.Th().Text("Key"),
		app.Th().Text("Value"),
	))

	return app.Div().Class("card").Body(
		app.Div().Body(
			app.Strong().Text(fmt.Sprintf("Node %d", node.PeerID)),
			app.Text(" "+node.Name),
		),
		app.Div().Class("grid meta").Body(
			app.Div().Text("Address: " + node.RemoteAddr),
			app.Div().Text("Last heartbeat: " + node.LastHeartbeatAgo + " (" + node.LastHeartbeat + ")"),
			app.Div().Text(rssi),
		),
		app.Table().Body(
			header,
			app.TBody().Body(rows...),
		),
	)
}

func panelStyles() app.UI {
	return app.Style().Text(panelCSS)
}

const panelCSS = `
:root {
	color-scheme: dark;
}
body {
	font-family: Arial, sans-serif;
	margin: 24px;
	background: #0b1220;
	color: #e5e7eb;
}
a {
	text-decoration: none;
	color: #e2e8f0;
}
h1 {
	margin: 0;
}
h2 {
	margin-top: 0;
}
.top {
	display: flex;
	justify-content: space-between;
	align-items: center;
	gap: 12px;
	margin-bottom: 16px;
}
.top-actions {
	display: flex;
	gap: 8px;
	align-items: center;
}
.card {
	background: #111827;
	border: 1px solid #1f2937;
	border-radius: 12px;
	padding: 14px;
	margin-bottom: 14px;
}
.row-title {
	color: #9ca3af;
	font-size: 0.9rem;
	margin-bottom: 8px;
	text-transform: uppercase;
	letter-spacing: 0.08em;
}
.row {
	display: flex;
	gap: 10px;
	flex-wrap: wrap;
	align-items: center;
}
.node {
	background: #0f172a;
	border: 1px solid #334155;
	border-radius: 10px;
	min-width: 160px;
	padding: 10px 12px;
}
.node.active {
	background: #172554;
	border-color: #2563eb;
}
.node-name {
	font-weight: 700;
}
.node-rssi {
	color: #93c5fd;
	margin-top: 4px;
}
.connections-list {
	margin: 0;
	padding-left: 18px;
}
button,
.admin-btn,
.top-actions a {
	border: 0;
	border-radius: 8px;
	padding: 8px 12px;
	background: #2563eb;
	color: white;
	font-weight: 600;
	cursor: pointer;
	display: inline-block;
}
button.secondary {
	background: #334155;
}
input {
	width: 100%;
	box-sizing: border-box;
	background: #0f172a;
	color: #e5e7eb;
	border: 1px solid #334155;
	border-radius: 8px;
	padding: 7px;
}
label {
	display: block;
	margin-top: 10px;
	margin-bottom: 6px;
	color: #9ca3af;
}
.subtle {
	color: #9ca3af;
	margin-top: 4px;
}
.grid {
	display: grid;
	gap: 8px;
	grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
	margin: 10px 0;
}
.meta {
	color: #9ca3af;
	font-size: 0.92rem;
}
table {
	width: 100%;
	border-collapse: collapse;
}
th,
td {
	border: 1px solid #1f2937;
	padding: 8px;
	text-align: left;
	vertical-align: top;
}
th {
	background: #0f172a;
}
.cfg-form {
	display: grid;
	grid-template-columns: 1fr auto;
	gap: 8px;
	align-items: center;
}
.empty {
	color: #6b7280;
	font-style: italic;
}
.login-wrap {
	min-height: calc(100vh - 48px);
	display: grid;
	place-items: center;
}
.login-card {
	width: min(420px, 92vw);
}
`
