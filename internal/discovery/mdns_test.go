package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

func TestAdvertiseAndDiscover(t *testing.T) {
	info := Info{ID: "beam-test-device", Name: "Windows-PC", Type: "windows", Port: 47821}
	adv, err := Advertise(info, false)
	if err != nil {
		t.Skipf("mDNS advertisement unavailable: %v", err)
	}
	defer adv.Close()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		found, err := Browse(context.Background(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		for _, remote := range found {
			if remote.ID == info.ID && remote.Name == info.Name && remote.Port == info.Port {
				return
			}
		}
	}
	t.Fatal("advertised BEAM device was not discovered")
}

func TestFromEntry(t *testing.T) {
	e := &zeroconf.ServiceEntry{}
	e.Instance = "MacBook"
	e.Port = 47821
	e.Text = []string{"id=abc", "name=MacBook", "type=mac", "proto=1", "pair=1"}
	e.AddrIPv4 = nil
	r, ok := fromEntry(e)
	if !ok || r.ID != "abc" || r.Name != "MacBook" || r.Type != "mac" || !r.Pair || r.Port != 47821 || r.Instance != "MacBook" {
		t.Fatalf("%+v %v", r, ok)
	}
}

func TestRenameKeepsOneAdvertisement(t *testing.T) {
	stale := Remote{ID: "X", Name: "Windows-PC", Instance: "Windows-PC", TTL: 3200}
	fresh := Remote{ID: "X", Name: "Suraj-Windows", Instance: "X", TTL: 3200}
	for _, got := range []Remote{keepRemote(stale, fresh), keepRemote(fresh, stale)} {
		if got.Name != "Suraj-Windows" || got.ID != "X" {
			t.Fatalf("kept %+v", got)
		}
	}
	older := keepRemote(fresh, Remote{ID: "X", Name: "Windows-PC", Instance: "Windows-PC", TTL: 100})
	if older.Name != "Suraj-Windows" {
		t.Fatalf("stale record replaced the current name: %+v", older)
	}
}
