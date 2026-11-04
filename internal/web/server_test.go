package web

import "testing"

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
