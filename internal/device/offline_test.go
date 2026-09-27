package device

import "testing"

func TestOfflineHandling(t *testing.T) {
	t.Setenv("BEAM_CONFIG_DIR", t.TempDir())
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	id, err := Init("MacBook")
	if err != nil {
		t.Fatal(err)
	}
	if err := UpsertPeer(Peer{ID: "deadbeefdeadbeefdeadbeefdeadbeef", Name: "iPhone", Type: TypeIOS}); err != nil {
		t.Fatal(err)
	}
	items, err := ListDevices(id, nil)
	if err != nil {
		t.Fatal(err)
	}
	tgt, err := ResolveTarget("iphone", items)
	if err != nil {
		t.Fatal(err)
	}
	if tgt.Status != "offline" || tgt.Addr != "" {
		t.Fatalf("%+v", tgt)
	}
}
