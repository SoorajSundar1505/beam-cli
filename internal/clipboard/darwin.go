//go:build darwin

package clipboard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"beam/internal/history"
)

type darwin struct{}

func native() Platform { return darwin{} }

func (darwin) Read() (*Item, error) {
	if files := pasteboardFiles(); len(files) > 0 {
		path := files[0]
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
	out, err := exec.Command("pbpaste").Output()
	if err != nil {
		return nil, err
	}
	text := string(out)
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	return TextItem(text), nil
}

func (darwin) Write(it *Item) error {
	if it == nil {
		return nil
	}
	if it.Kind == history.KindText {
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(it.Text)
		return cmd.Run()
	}
	name := it.Filename
	if name == "" {
		name = "clipboard.bin"
		if strings.HasPrefix(it.MIME, "image/") {
			name = "clipboard.png"
		}
	}
	dir, err := os.MkdirTemp("", "beam-clip-")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, it.Data, 0o600); err != nil {
		return err
	}
	script := `set the clipboard to POSIX file "` + escapeAS(path) + `"`
	return exec.Command("osascript", "-e", script).Run()
}

func pasteboardFiles() []string {
	out, err := exec.Command("osascript", "-e", `try
    set theFiles to (the clipboard as «class furl»)
    return POSIX path of theFiles
on error
    return ""
end try`).CombinedOutput()
	if err != nil {
		return nil
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return nil
	}
	if _, err := os.Stat(p); err != nil {
		return nil
	}
	return []string{p}
}

func escapeAS(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
