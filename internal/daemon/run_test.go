package daemon

import (
	"context"
	"testing"
	"time"

	"beam/internal/device"
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
