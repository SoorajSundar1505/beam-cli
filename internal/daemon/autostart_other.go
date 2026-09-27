//go:build !darwin && !windows

package daemon

import "fmt"

func EnableAutostart() error {
	return fmt.Errorf("automatic startup is currently supported on macOS and Windows")
}

func DisableAutostart() error {
	return nil
}
