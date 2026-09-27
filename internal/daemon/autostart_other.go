//go:build !darwin && !windows

package daemon

func EnableAutostart() error {
	return ErrAutostartUnsupported
}

func DisableAutostart() error {
	return nil
}

func AutostartEnabled() (bool, error) {
	return false, nil
}
