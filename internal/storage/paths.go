package storage

import (
	"os"
	"path/filepath"
	"runtime"
)

const appName = "beam"

func ConfigDir() (string, error) {
	if dir := os.Getenv("BEAM_CONFIG_DIR"); dir != "" {
		return dir, os.MkdirAll(dir, 0o700)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, appName)
	return dir, os.MkdirAll(dir, 0o700)
}

func DataDir() (string, error) {
	if dir := os.Getenv("BEAM_DATA_DIR"); dir != "" {
		return dir, os.MkdirAll(dir, 0o700)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	var dir string
	switch runtime.GOOS {
	case "darwin":
		dir = filepath.Join(home, "Library", "Application Support", appName)
	case "windows":
		if base, err := os.UserConfigDir(); err == nil {
			dir = filepath.Join(base, appName, "data")
		} else {
			dir = filepath.Join(home, "AppData", "Local", appName)
		}
	default:
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			dir = filepath.Join(xdg, appName)
		} else {
			dir = filepath.Join(home, ".local", "share", appName)
		}
	}
	return dir, os.MkdirAll(dir, 0o700)
}

func DownloadsDir() (string, error) {
	if dir := os.Getenv("BEAM_DOWNLOADS_DIR"); dir != "" {
		return dir, os.MkdirAll(dir, 0o755)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func KeyPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "device.key"), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func PeersPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "peers.json"), nil
}

func HistoryPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "clipboard.db"), nil
}

func BlobDir() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	b := filepath.Join(dir, "clipboard-blobs")
	return b, os.MkdirAll(b, 0o700)
}
