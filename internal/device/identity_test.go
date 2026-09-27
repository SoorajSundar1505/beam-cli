package device

import (
	"os"
	"testing"
	"time"

	"beam/internal/crypto"
	"beam/internal/discovery"
)

func TestInitAndLoad(t *testing.T) {
	t.Setenv("BEAM_CONFIG_DIR", t.TempDir())
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	id, err := Init("MacBook")
	if err != nil {
		t.Fatal(err)
	}
	if id.Config.Name != "MacBook" || id.Config.DeviceID == "" {
		t.Fatalf("bad config %+v", id.Config)
	}
	again, created, err := Ensure("other")
	if err != nil || created {
		t.Fatalf("existing device: created=%v err=%v", created, err)
	}
	if again.Config.Name != "other" || again.Config.DeviceID != id.Config.DeviceID {
		t.Fatalf("identity changed: %+v", again.Config)
	}
	unchanged, created, err := Ensure("")
	if err != nil || created || unchanged.Config.Name != "other" {
		t.Fatalf("blank ensure rewrote identity: %+v created=%v err=%v", unchanged, created, err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Config.DeviceID != id.Config.DeviceID {
		t.Fatal("id mismatch")
	}
	want := crypto.DeviceID(loaded.PublicKey)
	if want != loaded.Config.DeviceID {
		t.Fatal("id not derived from key")
	}
}

func TestPeersAndList(t *testing.T) {
	t.Setenv("BEAM_CONFIG_DIR", t.TempDir())
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	id, err := Init("MacBook")
	if err != nil {
		t.Fatal(err)
	}
	p := Peer{
		ID:        "abcd",
		Name:      "Windows-PC",
		Type:      TypeWindows,
		PublicKey: "key",
		PairedAt:  time.Now(),
	}
	if err := UpsertPeer(p); err != nil {
		t.Fatal(err)
	}
	items, err := ListDevices(id, []discovery.Remote{{
		ID: "abcd", Name: "Windows-PC", Type: string(TypeWindows), Addr: "127.0.0.1:9",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Self != true || items[0].Status != "this device" {
		t.Fatalf("self: %+v", items[0])
	}
	if items[1].Status != "online" {
		t.Fatalf("peer status %s", items[1].Status)
	}
	got, err := ResolveTarget("windows", items)
	if err != nil || got.Name != "Windows-PC" {
		t.Fatalf("resolve: %+v %v", got, err)
	}
	offline, err := ListDevices(id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if offline[1].Status != "offline" {
		t.Fatalf("expected offline, got %s", offline[1].Status)
	}
	_ = os.Stderr
}
