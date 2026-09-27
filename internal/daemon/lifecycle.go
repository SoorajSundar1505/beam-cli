package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"beam/internal/storage"
)

var ErrAlreadyRunning = errors.New("daemon is already running")

var (
	heartbeatFreshFor = 6 * time.Second
	startupTimeout    = 5 * time.Second
	pollInterval      = 100 * time.Millisecond
	launchBackground  = launchBackgroundProcess
)

type State struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`
	Heartbeat time.Time `json:"heartbeat"`
}

func Status() (State, bool, error) {
	path, err := storage.DaemonStatePath()
	if err != nil {
		return State{}, false, err
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return State{}, false, nil
	}
	if err != nil {
		return State{}, false, err
	}
	var state State
	if err := json.Unmarshal(b, &state); err != nil {
		return State{}, false, nil
	}
	running := state.PID > 0 && time.Since(state.Heartbeat) < heartbeatFreshFor
	return state, running, nil
}

func Start() error {
	if _, running, err := Status(); err != nil {
		return err
	} else if running {
		return ErrAlreadyRunning
	}

	lockPath, err := storage.DaemonStartLockPath()
	if err != nil {
		return err
	}
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		if _, running, _ := Status(); running {
			return ErrAlreadyRunning
		}
		return fmt.Errorf("daemon start is already in progress")
	}
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Close()
		_ = os.Remove(lockPath)
	}()

	// Recheck after taking the lock so concurrent starts cannot launch twice.
	if _, running, err := Status(); err != nil {
		return err
	} else if running {
		return ErrAlreadyRunning
	}
	statePath, _ := storage.DaemonStatePath()
	stopPath, _ := storage.DaemonStopPath()
	_ = os.Remove(statePath)
	_ = os.Remove(stopPath)

	logPath, err := storage.DaemonLogPath()
	if err != nil {
		return err
	}
	if err := launchBackground(logPath); err != nil {
		return err
	}

	deadline := time.Now().Add(startupTimeout)
	for time.Now().Before(deadline) {
		if _, running, _ := Status(); running {
			return nil
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("daemon did not start; see %s", logPath)
}

func launchBackgroundProcess(logPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "daemon")
	cmd.Stdin = nil
	cmd.Stdout = log
	cmd.Stderr = log
	detach(cmd)
	if err := cmd.Start(); err != nil {
		log.Close()
		return err
	}
	_ = cmd.Process.Release()
	_ = log.Close()
	return nil
}

func Stop() error {
	state, running, err := Status()
	if err != nil {
		return err
	}
	if !running {
		return nil
	}
	stopPath, err := storage.DaemonStopPath()
	if err != nil {
		return err
	}
	if err := os.WriteFile(stopPath, []byte("stop\n"), 0o600); err != nil {
		return err
	}
	path, _ := storage.DaemonStatePath()
	deadline := time.Now().Add(heartbeatFreshFor + time.Second)
	for time.Now().Before(deadline) {
		if _, running, _ := Status(); !running {
			_ = os.Remove(path)
			return nil
		}
		time.Sleep(pollInterval)
	}
	p, err := os.FindProcess(state.PID)
	if err != nil {
		return err
	}
	if err := terminate(p); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	_ = os.Remove(path)
	_ = os.Remove(stopPath)
	return nil
}

func writeState(ctx context.Context) error {
	path, err := storage.DaemonStatePath()
	if err != nil {
		return err
	}
	state := State{PID: os.Getpid(), StartedAt: time.Now().UTC()}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	defer os.Remove(path)
	for {
		state.Heartbeat = time.Now().UTC()
		b, _ := json.Marshal(state)
		tmp := path + ".partial"
		if err := os.WriteFile(tmp, b, 0o600); err == nil {
			_ = os.Rename(tmp, path)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(exe)
}
