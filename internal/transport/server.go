package transport

import (
	"errors"
	"strings"
)

// Reject pairing on the data listener: pairing uses a dedicated beam pair process.
func pairingAllowed(mode string, pairingCode string) error {
	if mode == "pair" && strings.TrimSpace(pairingCode) == "" {
		return errors.New("this device is not in pairing mode")
	}
	return nil
}
