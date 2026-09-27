package queue

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"beam/internal/protocol"
	"beam/internal/storage"
)

type Entry struct {
	ID         string    `json:"id"`
	TargetID   string    `json:"target_id"`
	TargetName string    `json:"target_name"`
	Name       string    `json:"name"`
	Payload    string    `json:"payload"`
	Size       int64     `json:"size"`
	CreatedAt  time.Time `json:"created_at"`
}

func EnqueueFile(path, targetID, targetName string) (*Entry, error) {
	name, err := protocol.SafeFileName(filepath.Base(path))
	if err != nil {
		return nil, err
	}
	src, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	st, err := src.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("only regular files can be queued")
	}
	dir, err := storage.QueueDir()
	if err != nil {
		return nil, err
	}
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	payload := filepath.Join(dir, id+".data")
	dst, err := os.OpenFile(payload+".partial", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		_ = os.Remove(payload + ".partial")
		return nil, err
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(payload + ".partial")
		return nil, err
	}
	if err := os.Rename(payload+".partial", payload); err != nil {
		_ = os.Remove(payload + ".partial")
		return nil, err
	}
	entry := &Entry{
		ID: id, TargetID: targetID, TargetName: targetName, Name: name,
		Payload: payload, Size: st.Size(), CreatedAt: time.Now().UTC(),
	}
	if err := writeEntry(dir, entry); err != nil {
		_ = os.Remove(payload)
		return nil, err
	}
	return entry, nil
}

func List() ([]Entry, error) {
	dir, err := storage.QueueDir()
	if err != nil {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var e Entry
		if json.Unmarshal(b, &e) == nil && e.ID != "" && strings.HasPrefix(filepath.Clean(e.Payload), filepath.Clean(dir)+string(os.PathSeparator)) {
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt.Before(entries[j].CreatedAt) })
	return entries, nil
}

func Remove(e Entry) error {
	dir, err := storage.QueueDir()
	if err != nil {
		return err
	}
	_ = os.Remove(e.Payload)
	err = os.Remove(filepath.Join(dir, e.ID+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func writeEntry(dir string, e *Entry) error {
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, e.ID+".json.partial")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, e.ID+".json"))
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
