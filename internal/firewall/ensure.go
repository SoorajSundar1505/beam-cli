package firewall

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"beam/internal/storage"
)

var run = platformRun

func Ensure(port int) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	return apply(port)
}

func Elevate(port int) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	return elevate(addArgs(port))
}

func RememberAsked() error {
	return writeMarker("firewall.asked")
}

func apply(port int) error {
	if markerExists("firewall.ok") {
		return nil
	}
	out, err := run(showArgs()...)
	if err == nil && rulePresent(out) {
		return writeMarker("firewall.ok")
	}
	out, err = run(addArgs(port)...)
	if err == nil {
		return writeMarker("firewall.ok")
	}
	if accessDenied(err, out) {
		if markerExists("firewall.asked") {
			return nil
		}
		return ErrNeedsElevation
	}
	return err
}

func rulePresent(out []byte) bool {
	text := string(out)
	if strings.Contains(strings.ToLower(text), "no rules match") {
		return false
	}
	return strings.Contains(text, ruleName)
}

func accessDenied(err error, out []byte) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error() + " " + string(out))
	return strings.Contains(text, "access is denied") || strings.Contains(text, "requires elevation")
}

func markerExists(name string) bool {
	path, err := markerPath(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func writeMarker(name string) error {
	path, err := markerPath(name)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte("ok\n"), 0o600)
}

func markerPath(name string) (string, error) {
	dir, err := storage.RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func platformRun(args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("missing command")
	}
	return platformExec(args[0], args[1:])
}
