package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"beam/internal/device"
)

func SelectDevice(in io.Reader, out io.Writer, items []device.Listed) (*device.Listed, error) {
	renderChoices(out, items)
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		return nil, fmt.Errorf("no device selected")
	}
	n, err := strconv.Atoi(strings.TrimSpace(sc.Text()))
	if err != nil {
		return nil, fmt.Errorf("invalid selection")
	}
	for i := range items {
		if items[i].Index == n {
			if items[i].Self {
				return nil, fmt.Errorf("cannot send to this device")
			}
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("invalid selection")
}

func MustIdent() *device.Identity {
	ident, err := device.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "beam: %v\n", err)
		os.Exit(1)
	}
	return ident
}
