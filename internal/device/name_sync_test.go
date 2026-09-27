package device

import (
	"bytes"
	"testing"
	"time"

	"beam/internal/discovery"
)

func TestAdvertisedNameUpdatesPairedDevice(t *testing.T) {
	cases := []struct {
		local    string
		before   string
		after    string
		remoteID string
		kind     Type
	}{
		{local: "MacBook", before: "Windows-PC", after: "Suraj-Windows", remoteID: "X", kind: TypeWindows},
		{local: "Windows-PC", before: "MacBook", after: "Suraj-Mac", remoteID: "Y", kind: TypeMac},
	}
	for _, tc := range cases {
		t.Run(tc.before, func(t *testing.T) {
			t.Setenv("BEAM_CONFIG_DIR", t.TempDir())
			t.Setenv("BEAM_DATA_DIR", t.TempDir())
			local, err := Init(tc.local)
			if err != nil {
				t.Fatal(err)
			}
			public := append([]byte(nil), local.PublicKey...)
			private := append([]byte(nil), local.PrivateKey...)
			pairedAt := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
			if err := UpsertPeer(Peer{
				ID: tc.remoteID, Name: tc.before, Type: tc.kind,
				PublicKey: "public-key-" + tc.remoteID, PairedAt: pairedAt,
			}); err != nil {
				t.Fatal(err)
			}

			online, err := ListDevices(local, []discovery.Remote{
				{ID: local.Config.DeviceID, Name: "not-the-local-name", Type: string(local.Config.Type)},
				{ID: tc.remoteID, Name: tc.after, Type: string(tc.kind), Addr: "192.168.1.36:47821"},
				{ID: "unpaired", Name: "Other-Device", Type: string(tc.kind)},
			})
			if err != nil {
				t.Fatal(err)
			}
			if online[0].Name != tc.local || !online[0].Self || online[0].ID != local.Config.DeviceID {
				t.Fatalf("local device: %+v", online[0])
			}
			peer := findListed(online, tc.remoteID)
			if peer.Name != tc.after || peer.Status != "online" || peer.Peer == nil || peer.Peer.PublicKey != "public-key-"+tc.remoteID {
				t.Fatalf("online peer: %+v", peer)
			}
			if stringsCount(online, tc.remoteID) != 1 || stringsCount(online, local.Config.DeviceID) != 1 {
				t.Fatalf("device list duplicated ids: %+v", online)
			}

			stored, err := LoadPeers()
			if err != nil {
				t.Fatal(err)
			}
			if len(stored) != 1 || stored[0].ID != tc.remoteID || stored[0].Name != tc.after {
				t.Fatalf("stored peers: %+v", stored)
			}
			if stored[0].PublicKey != "public-key-"+tc.remoteID || !stored[0].PairedAt.Equal(pairedAt) {
				t.Fatalf("pairing record changed: %+v", stored[0])
			}

			offline, err := ListDevices(local, nil)
			if err != nil {
				t.Fatal(err)
			}
			gone := findListed(offline, tc.remoteID)
			if gone.Name != tc.after || gone.Status != "offline" {
				t.Fatalf("offline peer: %+v", gone)
			}
			target, err := ResolveTarget(tc.after, online)
			if err != nil || target.ID != tc.remoteID || target.Peer.PublicKey != stored[0].PublicKey {
				t.Fatalf("resolve new name: %+v %v", target, err)
			}

			renamed, created, err := Ensure(localRename(tc.local))
			if err != nil || created {
				t.Fatalf("rename created=%v err=%v", created, err)
			}
			if renamed.Config.DeviceID != local.Config.DeviceID || renamed.Config.Name != localRename(tc.local) {
				t.Fatalf("local identity changed: %+v", renamed.Config)
			}
			if !bytes.Equal(renamed.PublicKey, public) || !bytes.Equal(renamed.PrivateKey, private) {
				t.Fatal("rename replaced the keypair")
			}
			shown, err := ListDevices(renamed, nil)
			if err != nil {
				t.Fatal(err)
			}
			if shown[0].Name != localRename(tc.local) || findListed(shown, tc.remoteID).Name != tc.after {
				t.Fatalf("names after local rename: %+v", shown)
			}
		})
	}
}

func localRename(name string) string {
	if name == "MacBook" {
		return "Suraj-Mac"
	}
	return "Suraj-Windows"
}

func findListed(items []Listed, id string) Listed {
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	return Listed{}
}

func stringsCount(items []Listed, id string) int {
	n := 0
	for _, item := range items {
		if item.ID == id {
			n++
		}
	}
	return n
}
