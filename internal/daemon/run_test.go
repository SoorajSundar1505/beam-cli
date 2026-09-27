package daemon

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"beam/internal/crypto"
	"beam/internal/device"
	"beam/internal/protocol"
	"beam/internal/transport"
)

func TestStatusReportsForegroundDaemonAndShutdown(t *testing.T) {
	t.Setenv("BEAM_CONFIG_DIR", t.TempDir())
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	t.Setenv("BEAM_DOWNLOADS_DIR", t.TempDir())

	ident, err := device.Init("daemon-test")
	if err != nil {
		t.Fatal(err)
	}
	// Let the OS choose a free port so this test cannot collide with a local
	// BEAM daemon or another test process.
	ident.Config.ListenPort = 0
	if err := device.SaveConfig(ident.Config); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx)
	}()

	waitForStatus(t, true)
	state, running, err := Status()
	if err != nil {
		t.Fatal(err)
	}
	if !running || state.PID == 0 {
		t.Fatalf("foreground daemon status: running=%v state=%+v", running, state)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not shut down after context cancellation")
	}
	waitForStatus(t, false)
}

func TestMacClientConnectsAndHandshakesWithWindowsDaemon(t *testing.T) {
	t.Setenv("BEAM_CONFIG_DIR", t.TempDir())
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	t.Setenv("BEAM_DOWNLOADS_DIR", t.TempDir())

	port := availableIPv4Port(t)
	windows, err := device.Init("Windows-PC")
	if err != nil {
		t.Fatal(err)
	}
	windows.Config.Type = device.TypeWindows
	windows.Config.ListenPort = port
	if err := device.SaveConfig(windows.Config); err != nil {
		t.Fatal(err)
	}

	macPublic, macPrivate, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	mac := &device.Identity{
		Config: device.Config{
			DeviceID: crypto.DeviceID(macPublic),
			Name:     "MacBook",
			Type:     device.TypeMac,
		},
		PrivateKey: macPrivate,
		PublicKey:  macPublic,
	}
	if err := device.UpsertPeer(device.Peer{
		ID: mac.Config.DeviceID, Name: mac.Config.Name, Type: mac.Config.Type,
		PublicKey: crypto.EncodePublicKey(mac.PublicKey),
	}); err != nil {
		t.Fatal(err)
	}
	windowsPeer := &device.Peer{
		ID: windows.Config.DeviceID, Name: windows.Config.Name, Type: windows.Config.Type,
		PublicKey: crypto.EncodePublicKey(windows.PublicKey),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx) }()
	waitForStatus(t, true)

	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	raw, err := net.DialTimeout("tcp4", address, 3*time.Second)
	if err != nil {
		cancel()
		t.Fatalf("IPv4 connection to daemon listener %s failed: %v", address, err)
	}
	conn, err := transport.HandshakeInitiator(raw, mac, protocol.ModeData, "", windowsPeer)
	if err != nil {
		raw.Close()
		cancel()
		t.Fatalf("BEAM handshake with daemon failed: %v", err)
	}
	if conn.Remote.DeviceID != windows.Config.DeviceID {
		t.Fatalf("connected to device %q, want %q", conn.Remote.DeviceID, windows.Config.DeviceID)
	}
	_ = conn.Close()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not shut down")
	}
}

func availableIPv4Port(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func waitForStatus(t *testing.T, want bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, running, err := Status()
		if err != nil {
			t.Fatal(err)
		}
		if running == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("daemon running status did not become %v", want)
}
