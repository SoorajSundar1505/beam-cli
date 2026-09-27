package device

import (
	"testing"
	"time"

	"beam/internal/discovery"
)

func TestPeerEndpointRefreshKeepsOnePairing(t *testing.T) {
	t.Setenv("BEAM_CONFIG_DIR", t.TempDir())
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	local, err := Init("Suraj-Mac")
	if err != nil {
		t.Fatal(err)
	}
	pairedAt := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	key := "public-key-windows"
	if err := UpsertPeer(Peer{
		ID: "win", Name: "Windows-PC", Type: TypeWindows,
		PublicKey: key, PairedAt: pairedAt, Endpoint: "192.168.1.20:47821",
	}); err != nil {
		t.Fatal(err)
	}

	renamed, err := ListDevices(local, []discovery.Remote{{
		ID: "win", Name: "Suraj-Windows", Type: string(TypeWindows), Addr: "192.168.1.20:47821",
	}})
	if err != nil {
		t.Fatal(err)
	}
	peer := findListed(renamed, "win")
	if peer.Name != "Suraj-Windows" || peer.Status != "online" || stringsCount(renamed, "win") != 1 {
		t.Fatalf("renamed peer: %+v count=%d", peer, stringsCount(renamed, "win"))
	}

	moved, err := ListDevices(local, []discovery.Remote{{
		ID: "win", Name: "Suraj-Windows", Type: string(TypeWindows), Addr: "192.168.1.50:47821",
	}})
	if err != nil {
		t.Fatal(err)
	}
	peer = findListed(moved, "win")
	if peer.Addr != "192.168.1.50:47821" || peer.Status != "online" || peer.Name != "Suraj-Windows" {
		t.Fatalf("moved peer: %+v", peer)
	}
	stored, err := LoadPeers()
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].PublicKey != key || !stored[0].PairedAt.Equal(pairedAt) {
		t.Fatalf("pairing changed: %+v", stored)
	}
	if stored[0].Endpoint != "192.168.1.50:47821" || stored[0].LastSeen.IsZero() {
		t.Fatalf("endpoint freshness: %+v", stored[0])
	}
	if stringsCount(moved, "win") != 1 {
		t.Fatal("address change created a duplicate peer")
	}

	if err := MarkUnreachable("win", "192.168.1.50:47821"); err != nil {
		t.Fatal(err)
	}
	stale, err := ListDevices(local, []discovery.Remote{{
		ID: "win", Name: "Suraj-Windows", Type: string(TypeWindows), Addr: "192.168.1.50:47821",
	}})
	if err != nil {
		t.Fatal(err)
	}
	peer = findListed(stale, "win")
	if peer.Status != "unreachable" || peer.Peer.PublicKey != key {
		t.Fatalf("stale endpoint: %+v", peer)
	}

	fresh, err := ListDevices(local, []discovery.Remote{{
		ID: "win", Name: "Suraj-Windows", Type: string(TypeWindows), Addr: "10.0.0.8:47821",
	}})
	if err != nil {
		t.Fatal(err)
	}
	peer = findListed(fresh, "win")
	if peer.Status != "online" || peer.Addr != "10.0.0.8:47821" || peer.Peer.PublicKey != key || peer.Name != "Suraj-Windows" {
		t.Fatalf("refreshed endpoint: %+v", peer)
	}
	stored, err = LoadPeers()
	if err != nil {
		t.Fatal(err)
	}
	if stored[0].BadEndpoint != "" || stored[0].Endpoint != "10.0.0.8:47821" || !stored[0].PairedAt.Equal(pairedAt) {
		t.Fatalf("pairing did not survive the new endpoint: %+v", stored[0])
	}

	if err := MarkReachable("win", "10.0.0.8:47821"); err != nil {
		t.Fatal(err)
	}
	stored, err = LoadPeers()
	if err != nil {
		t.Fatal(err)
	}
	if stored[0].LastReachable.IsZero() || stored[0].PublicKey != key {
		t.Fatalf("reachable record: %+v", stored[0])
	}
}
