package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"beam/internal/daemon"
	"beam/internal/device"
	"beam/internal/discovery"
)

func TestInitEnablesAutostartAndStartsDaemon(t *testing.T) {
	setupCLIDaemonTest(t)
	var starts, enables int
	startDaemon = func() error { starts++; return nil }
	enableDaemonAutostart = func() error { enables++; return nil }

	output, err := execute(t, "init", "--name", "MacBook")
	if err != nil {
		t.Fatal(err)
	}
	ident, err := device.Load()
	if err != nil {
		t.Fatal(err)
	}
	if ident.Config.Name != "MacBook" {
		t.Fatalf("device name = %q", ident.Config.Name)
	}
	if starts != 1 || enables != 1 {
		t.Fatalf("starts=%d enables=%d, want 1 each", starts, enables)
	}
	for _, want := range []string{
		"ok BEAM is ready",
		"ok Background receiver started",
		"ok Autostart enabled",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q:\n%s", want, output)
		}
	}
}

func TestStartStopAndAutostartDisable(t *testing.T) {
	setupCLIDaemonTest(t)
	if _, err := device.Init("Windows-PC"); err != nil {
		t.Fatal(err)
	}
	var starts, stops, disables int
	startDaemon = func() error { starts++; return nil }
	stopDaemon = func() error { stops++; return nil }
	disableDaemonAutostart = func() error { disables++; return nil }

	if _, err := execute(t, "start"); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t, "stop"); err != nil {
		t.Fatal(err)
	}
	if disables != 0 {
		t.Fatal("plain stop disabled autostart")
	}
	if _, err := execute(t, "stop", "--disable-autostart"); err != nil {
		t.Fatal(err)
	}
	if starts != 1 || stops != 2 || disables != 1 {
		t.Fatalf("starts=%d stops=%d disables=%d", starts, stops, disables)
	}
}

func TestDevicesFirstRunIsIdempotent(t *testing.T) {
	setupCLIDaemonTest(t)
	var starts int
	running := false
	startDaemon = func() error {
		if running {
			return daemon.ErrAlreadyRunning
		}
		running = true
		starts++
		return nil
	}
	enableDaemonAutostart = func() error { return nil }
	browseDevices = func(context.Context) ([]discovery.Remote, error) {
		return []discovery.Remote{{ID: "peer", Name: "Windows-PC", Type: "windows"}}, nil
	}

	first, err := execute(t, "devices")
	if err != nil {
		t.Fatal(err)
	}
	second, err := execute(t, "devices")
	if err != nil {
		t.Fatal(err)
	}
	if starts != 1 {
		t.Fatalf("daemon starts = %d, want 1", starts)
	}
	ident, err := device.Load()
	if err != nil || ident.Config.DeviceID == "" {
		t.Fatalf("identity was not created: %+v %v", ident, err)
	}
	for _, output := range []string{first, second} {
		for _, want := range []string{"BEAM * ONLINE", "Nearby devices", "1. Windows-PC", "* online"} {
			if !strings.Contains(output, want) {
				t.Errorf("output missing %q:\n%s", want, output)
			}
		}
	}
	if strings.Contains(second, "ok BEAM is ready") {
		t.Fatal("repeated command repeated first-run setup")
	}
}

func TestRepeatedInitDoesNotError(t *testing.T) {
	setupCLIDaemonTest(t)
	if _, err := execute(t, "init", "--name", "MacBook"); err != nil {
		t.Fatal(err)
	}
	output, err := execute(t, "init", "--name", "MacBook")
	if err != nil {
		t.Fatalf("repeated init returned error: %v\n%s", err, output)
	}
	if strings.Contains(output, "already initialized") {
		t.Fatalf("repeated init treated existing identity as an error:\n%s", output)
	}
}

func TestStatusReporting(t *testing.T) {
	setupCLIDaemonTest(t)
	if _, err := device.Init("MacBook"); err != nil {
		t.Fatal(err)
	}
	daemonStatus = func() (daemon.State, bool, error) {
		return daemon.State{PID: 4242, StartedAt: time.Now(), Heartbeat: time.Now()}, true, nil
	}
	daemonAutostartEnabled = func() (bool, error) { return true, nil }
	browseDevices = func(context.Context) ([]discovery.Remote, error) {
		return []discovery.Remote{{ID: "peer", Name: "Windows-PC", Type: "windows"}}, nil
	}

	output, err := execute(t, "status")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"BEAM * ONLINE",
		"Device: MacBook",
		"Daemon: running (PID 4242)",
		"Autostart: enabled",
		"1. Windows-PC",
		"* online",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("status missing %q:\n%s", want, output)
		}
	}
}

func execute(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRoot()
	root.SetArgs(args)
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.Execute()
	return out.String(), err
}

func setupCLIDaemonTest(t *testing.T) {
	t.Helper()
	t.Setenv("BEAM_CONFIG_DIR", t.TempDir())
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	t.Setenv("BEAM_DOWNLOADS_DIR", t.TempDir())

	oldStart := startDaemon
	oldStop := stopDaemon
	oldStatus := daemonStatus
	oldEnable := enableDaemonAutostart
	oldDisable := disableDaemonAutostart
	oldEnabled := daemonAutostartEnabled
	oldBrowse := browseDevices
	startDaemon = func() error { return nil }
	stopDaemon = func() error { return nil }
	enableDaemonAutostart = func() error { return nil }
	disableDaemonAutostart = func() error { return nil }
	daemonAutostartEnabled = func() (bool, error) { return true, nil }
	daemonStatus = func() (daemon.State, bool, error) {
		return daemon.State{PID: 1, Heartbeat: time.Now()}, true, nil
	}
	browseDevices = func(context.Context) ([]discovery.Remote, error) { return nil, nil }
	t.Cleanup(func() {
		startDaemon = oldStart
		stopDaemon = oldStop
		daemonStatus = oldStatus
		enableDaemonAutostart = oldEnable
		disableDaemonAutostart = oldDisable
		daemonAutostartEnabled = oldEnabled
		browseDevices = oldBrowse
	})
}
