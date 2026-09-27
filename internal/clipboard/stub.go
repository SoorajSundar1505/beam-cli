//go:build !darwin && !windows

package clipboard

import "fmt"

type stub struct{}

func native() Platform { return stub{} }

func (stub) Read() (*Item, error) {
	return nil, fmt.Errorf("clipboard is not supported on this OS in Phase 1")
}

func (stub) Write(*Item) error {
	return fmt.Errorf("clipboard is not supported on this OS in Phase 1")
}
