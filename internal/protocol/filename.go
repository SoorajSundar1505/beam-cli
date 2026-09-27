package protocol

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func SafeFileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("empty filename")
	}
	if strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("invalid filename")
	}
	base := filepath.Base(name)
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == ".." {
		return "", fmt.Errorf("invalid filename")
	}
	if strings.ContainsAny(base, `/\`) {
		return "", fmt.Errorf("path separators not allowed")
	}
	if !utf8.ValidString(base) {
		return "", fmt.Errorf("filename is not valid UTF-8")
	}
	return base, nil
}
