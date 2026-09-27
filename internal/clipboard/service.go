package clipboard

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"beam/internal/history"
	"beam/internal/storage"
)

const MaxClipBytes = 32 << 20

type Item struct {
	Kind     history.Kind
	Text     string
	MIME     string
	Filename string
	Data     []byte
}

type Platform interface {
	Read() (*Item, error)
	Write(*Item) error
}

func fingerprint(it *Item) string {
	if it == nil {
		return ""
	}
	if it.Kind == history.KindText {
		return fmt.Sprintf("text|%s", it.Text)
	}
	sum := sha256.Sum256(it.Data)
	return fmt.Sprintf("%s|%s|%x", it.Kind, it.Filename, sum[:8])
}

type Service struct {
	p     Platform
	store *history.Store
	last  string
}

func New(p Platform, store *history.Store) *Service {
	return &Service{p: p, store: store}
}

func (s *Service) ReadCurrent() (*Item, error) {
	if s == nil || s.p == nil {
		return nil, fmt.Errorf("clipboard is not available on this platform")
	}
	item, err := s.p.Read()
	if err != nil {
		return nil, err
	}
	if item == nil || (item.Kind == history.KindText && strings.TrimSpace(item.Text) == "" && len(item.Data) == 0) {
		return nil, fmt.Errorf("clipboard is empty")
	}
	return item, nil
}

func (s *Service) persist(it *Item) (*history.Item, error) {
	rec := history.Item{
		Kind:      it.Kind,
		Text:      it.Text,
		MIME:      it.MIME,
		Filename:  it.Filename,
		Size:      int64(len(it.Data)),
		CreatedAt: time.Now(),
	}
	if it.Kind == history.KindText {
		rec.Size = int64(len(it.Text))
	} else if len(it.Data) > 0 {
		dir, err := storage.BlobDir()
		if err != nil {
			return nil, err
		}
		f, err := os.CreateTemp(dir, "clip-")
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(it.Data); err != nil {
			f.Close()
			_ = os.Remove(f.Name())
			return nil, err
		}
		if err := f.Close(); err != nil {
			_ = os.Remove(f.Name())
			return nil, err
		}
		rec.BlobPath = f.Name()
		rec.Size = int64(len(it.Data))
	}
	id, err := s.store.Add(rec)
	if err != nil {
		return nil, err
	}
	rec.ID = id
	return &rec, nil
}

func (s *Service) RecordIncoming(it *Item) (*history.Item, error) {
	return s.persist(it)
}

func (s *Service) Write(it *Item) error {
	if s.p == nil {
		return fmt.Errorf("clipboard is not available on this platform")
	}
	return s.p.Write(it)
}

func (s *Service) ItemFromHistory(h *history.Item) (*Item, error) {
	it := &Item{
		Kind:     h.Kind,
		Text:     h.Text,
		MIME:     h.MIME,
		Filename: h.Filename,
	}
	if h.BlobPath != "" {
		b, err := os.ReadFile(h.BlobPath)
		if err != nil {
			return nil, err
		}
		it.Data = b
	}
	return it, nil
}

func Preview(it history.Item) string {
	switch it.Kind {
	case history.KindImage, history.KindFile:
		if it.Filename != "" {
			return it.Filename
		}
		return string(it.Kind)
	default:
		t := strings.ReplaceAll(it.Text, "\n", " ")
		t = strings.TrimSpace(t)
		if utf8.RuneCountInString(t) > 40 {
			r := []rune(t)
			t = string(r[:40]) + "…"
		}
		if t == "" {
			return "(empty)"
		}
		return t
	}
}

func TextItem(s string) *Item {
	return &Item{Kind: history.KindText, Text: s, MIME: "text/plain"}
}

func EqualText(a, b string) bool {
	return bytes.Equal([]byte(a), []byte(b))
}
