package portal

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
)

// TrayItem is a file or folder the hand can grab after it has been imported.
type TrayItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	Path string `json:"-"`
}

func loadTray(dir string) []TrayItem {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var items []TrayItem
	for _, entry := range entries {
		name := entry.Name()
		if name == "" || name[0] == '.' {
			continue
		}
		kind := "file"
		if entry.IsDir() {
			kind = "folder"
		}
		path := filepath.Join(dir, name)
		sum := sha256.Sum256([]byte(path))
		items = append(items, TrayItem{ID: hex.EncodeToString(sum[:8]), Name: name, Kind: kind, Path: path})
		if len(items) == 24 {
			break
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (item TrayItem) files() []string {
	if item.Kind != "folder" {
		if item.Path == "" {
			return nil
		}
		return []string{item.Path}
	}
	entries, err := os.ReadDir(item.Path)
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "" || entry.Name()[0] == '.' {
			continue
		}
		paths = append(paths, filepath.Join(item.Path, entry.Name()))
	}
	sort.Strings(paths)
	return paths
}
