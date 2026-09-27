package cli

import (
	"bytes"
	"context"
	"fmt"
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
		for _, want := range []string{"BEAM [ONLINE]", "Devices", "[THIS DEVICE]", "Windows-PC", "[ONLINE]"} {
			if !strings.Contains(output, want) {
				t.Errorf("output missing %q:\n%s", want, output)
			}
		}
		for _, hidden := range []string{"Starting daemon", "Waiting for daemon", "Launching background", "Checking PID", "ok BEAM is ready"} {
			if strings.Contains(output, hidden) {
				t.Errorf("output printed startup noise %q:\n%s", hidden, output)
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
		"BEAM [ONLINE]  MacBook",
		"daemon: running | autostart: on",
		"Devices",
		"[THIS DEVICE]",
		"Windows-PC",
		"[ONLINE]",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("status missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "4242") || strings.Contains(output, "Queued") || strings.Contains(output, "pid:") {
		t.Fatalf("normal status includes diagnostics:\n%s", output)
	}

	verbose, err := execute(t, "status", "--verbose")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"pid: 4242", "queued: 0", "device id:", "config:", "log:"} {
		if !strings.Contains(verbose, want) {
			t.Errorf("verbose status missing %q:\n%s", want, verbose)
		}
	}
}

func TestCommandsWakeOneDaemon(t *testing.T) {
	setupCLIDaemonTest(t)
	ready := &daemonGate{}
	ready.install(t)
	var enables int
	enableDaemonAutostart = func() error { enables++; return nil }
	browseDevices = func(context.Context) ([]discovery.Remote, error) {
		if !ready.running {
			t.Fatal("command continued before the daemon was ready")
		}
		ready.order = append(ready.order, "browse")
		return []discovery.Remote{{ID: "peer", Name: "Windows-PC", Type: "windows"}}, nil
	}

	if _, err := execute(t, "devices"); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t, "status"); err != nil {
		t.Fatal(err)
	}
	if _, err := execute(t); err != nil {
		t.Fatal(err)
	}
	if ready.starts != 1 || enables != 1 {
		t.Fatalf("starts=%d enables=%d, want one daemon and one autostart setup", ready.starts, enables)
	}
	if len(ready.order) == 0 || ready.order[0] != "start" {
		t.Fatalf("daemon was not ready before discovery: %v", ready.order)
	}
}

func TestStatusStartsStoppedDaemon(t *testing.T) {
	setupCLIDaemonTest(t)
	if _, err := device.Init("MacBook"); err != nil {
		t.Fatal(err)
	}
	ready := &daemonGate{}
	ready.install(t)
	browseDevices = func(context.Context) ([]discovery.Remote, error) {
		if !ready.running {
			t.Fatal("status listed devices before the daemon was ready")
		}
		return []discovery.Remote{{ID: "peer", Name: "Windows-PC", Type: "windows"}}, nil
	}
	output, err := execute(t, "status")
	if err != nil {
		t.Fatal(err)
	}
	if ready.starts != 1 || !ready.running {
		t.Fatalf("status starts=%d running=%v", ready.starts, ready.running)
	}
	for _, want := range []string{"BEAM [ONLINE]  MacBook", "daemon: running | autostart: on", "[THIS DEVICE]"} {
		if !strings.Contains(output, want) {
			t.Errorf("status missing %q:\n%s", want, output)
		}
	}
}

func TestStopThenNextCommandRestartsDaemon(t *testing.T) {
	setupCLIDaemonTest(t)
	if _, err := device.Init("MacBook"); err != nil {
		t.Fatal(err)
	}
	ready := &daemonGate{running: true}
	ready.install(t)
	var disables, enables int
	disableDaemonAutostart = func() error { disables++; return nil }
	enableDaemonAutostart = func() error { enables++; return nil }
	daemonAutostartEnabled = func() (bool, error) { return true, nil }

	if _, err := execute(t, "stop"); err != nil {
		t.Fatal(err)
	}
	if ready.running || ready.starts != 0 || disables != 0 {
		t.Fatalf("stop running=%v starts=%d disables=%d", ready.running, ready.starts, disables)
	}
	output, err := execute(t, "devices")
	if err != nil {
		t.Fatal(err)
	}
	if ready.starts != 1 || !ready.running || enables != 0 || disables != 0 {
		t.Fatalf("restart starts=%d running=%v enables=%d disables=%d", ready.starts, ready.running, enables, disables)
	}
	if !strings.Contains(output, "BEAM [ONLINE]") || !strings.Contains(output, "[THIS DEVICE]") {
		t.Fatalf("devices after stop:\n%s", output)
	}
}

func TestDevicesDoesNotDuplicateCurrentDevice(t *testing.T) {
	setupCLIDaemonTest(t)
	ident, err := device.Init("MacBook")
	if err != nil {
		t.Fatal(err)
	}
	if err := device.UpsertPeer(device.Peer{ID: "work", Name: "Work-Mac", Type: device.TypeMac, PublicKey: "key"}); err != nil {
		t.Fatal(err)
	}
	ready := &daemonGate{running: true}
	ready.install(t)
	browseDevices = func(context.Context) ([]discovery.Remote, error) {
		return []discovery.Remote{
			{ID: ident.Config.DeviceID, Name: "MacBook", Type: "mac"},
			{ID: "peer", Name: "Windows-PC", Type: "windows"},
		}, nil
	}
	output, err := execute(t, "devices")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(output, "MacBook") != 2 || strings.Count(output, "[THIS DEVICE]") != 1 {
		t.Fatalf("current device was duplicated or omitted:\n%s", output)
	}
	if !strings.Contains(output, "Windows-PC") || !strings.Contains(output, "[ONLINE]") || !strings.Contains(output, "Work-Mac") || !strings.Contains(output, "[OFFLINE]") {
		t.Fatalf("remote devices missing:\n%s", output)
	}
}

func TestDaemonStartupFailureIsReported(t *testing.T) {
	setupCLIDaemonTest(t)
	startDaemon = func() error { return fmt.Errorf("daemon did not start; see log") }
	output, err := execute(t, "status")
	if err == nil || !strings.Contains(err.Error(), "daemon did not start") {
		t.Fatalf("err=%v output=%s", err, output)
	}
	if strings.Contains(output, "Starting daemon") || strings.Contains(output, "Waiting for daemon") {
		t.Fatalf("startup noise:\n%s", output)
	}
}

type daemonGate struct {
	running bool
	starts  int
	order   []string
}

func (g *daemonGate) install(t *testing.T) {
	t.Helper()
	startDaemon = func() error {
		g.order = append(g.order, "start")
		if g.running {
			return daemon.ErrAlreadyRunning
		}
		g.starts++
		g.running = true
		return nil
	}
	stopDaemon = func() error {
		g.running = false
		return nil
	}
	daemonStatus = func() (daemon.State, bool, error) {
		if !g.running {
			return daemon.State{}, false, nil
		}
		return daemon.State{PID: 9, Heartbeat: time.Now()}, true, nil
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
