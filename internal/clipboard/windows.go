//go:build windows

package clipboard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"beam/internal/history"
)

type windows struct{}

func native() Platform { return windows{} }

func (windows) Read() (*Item, error) {
	out, err := exec.Command("powershell", "-NoProfile", "-Command",
		`Get-Clipboard -Format FileDropList | ForEach-Object { $_.FullName }`).Output()
	if err == nil {
		path := strings.TrimSpace(strings.Split(string(out), "\n")[0])
		path = strings.TrimRight(path, "\r")
		if path != "" {
			b, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			if int64(len(b)) > MaxClipBytes {
				return nil, nil
			}
			kind := history.KindFile
			mime := mimeFromName(path)
			if strings.HasPrefix(mime, "image/") {
				kind = history.KindImage
			}
			return &Item{Kind: kind, Filename: filepath.Base(path), MIME: mime, Data: b}, nil
		}
	}
	out, err = exec.Command("powershell", "-NoProfile", "-Command", "Get-Clipboard -Raw").Output()
	if err != nil {
		return nil, err
	}
	text := strings.TrimRight(string(out), "\r\n")
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	return TextItem(text), nil
}

func (windows) Write(it *Item) error {
	if it == nil {
		return nil
	}
	if it.Kind == history.KindText {
		cmd := exec.Command("powershell", "-NoProfile", "-Command", "Set-Clipboard -Value $input")
		cmd.Stdin = strings.NewReader(it.Text)
		return cmd.Run()
	}
	name := it.Filename
	if name == "" {
		name = "clipboard.bin"
	}
	dir, err := os.MkdirTemp("", "beam-clip-")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, it.Data, 0o600); err != nil {
		return err
	}
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "Set-Clipboard -Path $input")
	cmd.Stdin = strings.NewReader(path)
	return cmd.Run()
}
