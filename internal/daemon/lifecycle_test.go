package daemon

import (
	"encoding/json"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"beam/internal/storage"
)

func TestStartAndDuplicateStart(t *testing.T) {
	setupLifecycleTest(t)
	var launches atomic.Int32
	launchBackground = fakeLauncher(t, &launches, false)

	if err := Start(); err != nil {
		t.Fatal(err)
	}
	if err := Start(); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("duplicate start returned %v", err)
	}
	if got := launches.Load(); got != 1 {
		t.Fatalf("launched %d daemons, want 1", got)
	}
	if err := Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestStopTerminatesDaemon(t *testing.T) {
	setupLifecycleTest(t)
	var launches atomic.Int32
	launchBackground = fakeLauncher(t, &launches, false)

	if err := Start(); err != nil {
		t.Fatal(err)
	}
	if err := Stop(); err != nil {
		t.Fatal(err)
	}
	if _, running, err := Status(); err != nil || running {
		t.Fatalf("status after stop: running=%v err=%v", running, err)
	}
}

func TestCrashDoesNotRestartDaemon(t *testing.T) {
	setupLifecycleTest(t)
	var launches atomic.Int32
	launchBackground = fakeLauncher(t, &launches, true)

	if err := Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(heartbeatFreshFor + 50*time.Millisecond)
	if _, running, err := Status(); err != nil || running {
		t.Fatalf("status after crash: running=%v err=%v", running, err)
	}
	time.Sleep(100 * time.Millisecond)
	if got := launches.Load(); got != 1 {
		t.Fatalf("crashed daemon restarted %d times", got)
	}
}

func setupLifecycleTest(t *testing.T) {
	t.Helper()
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	oldLaunch := launchBackground
	oldFresh := heartbeatFreshFor
	oldTimeout := startupTimeout
	oldPoll := pollInterval
	heartbeatFreshFor = 200 * time.Millisecond
	startupTimeout = time.Second
	pollInterval = 10 * time.Millisecond
	t.Cleanup(func() {
		launchBackground = oldLaunch
		heartbeatFreshFor = oldFresh
		startupTimeout = oldTimeout
		pollInterval = oldPoll
	})
}

func fakeLauncher(t *testing.T, launches *atomic.Int32, crash bool) func(string) error {
	t.Helper()
	return func(string) error {
		launches.Add(1)
		path, err := storage.DaemonStatePath()
		if err != nil {
			return err
		}
		write := func() {
			state := State{PID: os.Getpid(), StartedAt: time.Now().UTC(), Heartbeat: time.Now().UTC()}
			b, _ := json.Marshal(state)
			_ = os.WriteFile(path, b, 0o600)
		}
		write()
		if crash {
			return nil
		}
		stopPath, _ := storage.DaemonStopPath()
		go func() {
			ticker := time.NewTicker(25 * time.Millisecond)
			defer ticker.Stop()
			for range ticker.C {
				if _, err := os.Stat(stopPath); err == nil {
					_ = os.Remove(stopPath)
					_ = os.Remove(path)
					return
				}
				write()
			}
		}()
		return nil
	}
}
