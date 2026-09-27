package daemon

import (
	"bytes"
	"testing"

	"beam/internal/device"
)

func TestConfiguredDisplayNameDoesNotReplaceIdentity(t *testing.T) {
	t.Setenv("BEAM_CONFIG_DIR", t.TempDir())
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	running, err := device.Init("Windows-PC")
	if err != nil {
		t.Fatal(err)
	}
	public := append([]byte(nil), running.PublicKey...)
	private := append([]byte(nil), running.PrivateKey...)
	id := running.Config.DeviceID

	if _, created, err := device.Ensure("Suraj-Windows"); err != nil || created {
		t.Fatalf("rename created=%v err=%v", created, err)
	}
	if !applyConfiguredName(running) {
		t.Fatal("daemon did not adopt the display name")
	}
	if running.Config.Name != "Suraj-Windows" || running.Config.DeviceID != id {
		t.Fatalf("daemon identity: %+v", running.Config)
	}
	if !bytes.Equal(running.PublicKey, public) || !bytes.Equal(running.PrivateKey, private) {
		t.Fatal("daemon keypair changed with the display name")
	}
	if applyConfiguredName(running) {
		t.Fatal("unchanged display name was treated as a new advertisement")
	}

	info := advertisement(running)
	if info.ID != id || info.Name != "Suraj-Windows" || info.Port != device.DefaultPort {
		t.Fatalf("advertisement: %+v", info)
	}
}
