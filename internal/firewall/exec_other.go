//go:build !windows

package firewall

import "errors"

func platformExec(name string, args []string) ([]byte, error) {
	return nil, errors.New("firewall setup is windows-only")
}

func elevate(args []string) error {
	return nil
}
