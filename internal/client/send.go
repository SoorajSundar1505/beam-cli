package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"beam/internal/clipboard"
	"beam/internal/device"
	"beam/internal/discovery"
	"beam/internal/history"
	"beam/internal/protocol"
	"beam/internal/server"
	"beam/internal/transfer"
	"beam/internal/transport"
)

var ErrOffline = errors.New("device is offline")

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

const maxDialAttempts = 2

const dialTimeout = 4 * time.Second

// UnreachableError is the user-facing result when a paired device cannot be
// connected. Cause keeps the dial or handshake error for debug output.
type UnreachableError struct {
	Name  string
	Cause error
}

func (e *UnreachableError) Error() string {
	return e.Name + " is currently unreachable."
}

func (e *UnreachableError) Unwrap() error { return e.Cause }

func DialTarget(ctx context.Context, ident *device.Identity, target *device.Listed) (*transport.Conn, error) {
	if target.Self {
		return nil, fmt.Errorf("cannot send to this device")
	}
	if target.Peer == nil {
		return nil, fmt.Errorf("device %s is not paired; run beam pair", target.Name)
	}
	return connectPeer(ctx, target, func(ctx context.Context, id string) (string, error) {
		remote, err := discovery.FindID(ctx, id)
		if err != nil {
			return "", err
		}
		return remote.Addr, nil
	}, func(ctx context.Context, addr string) (*transport.Conn, error) {
		return server.DialPeer(ctx, ident, target.Peer, addr)
	})
}

// connectPeer dials the current endpoint into the existing handshake. A failure
// drops that address, asks discovery for a newer one, and tries at most once more.
func connectPeer(ctx context.Context, target *device.Listed, lookup func(context.Context, string) (string, error), dial func(context.Context, string) (*transport.Conn, error)) (*transport.Conn, error) {
	addr := ""
	if target.Status != "offline" && target.Status != "unreachable" {
		addr = target.Addr
	}
	var cause error
	for attempt := 0; attempt < maxDialAttempts; attempt++ {
		if attempt > 0 || addr == "" {
			next, err := lookup(ctx, target.ID)
			if err == nil && next != "" {
				addr = next
			}
		}
		if addr == "" {
			break
		}
		dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
		conn, err := dial(dialCtx, addr)
		cancel()
		if err == nil {
			_ = device.MarkReachable(target.ID, addr)
			return conn, nil
		}
		cause = err
		_ = device.MarkUnreachable(target.ID, addr)
		addr = ""
	}
	if cause == nil {
		return nil, fmt.Errorf("%s: %w", target.Name, ErrOffline)
	}
	return nil, &UnreachableError{Name: target.Name, Cause: cause}
}

func SendFile(ctx context.Context, ident *device.Identity, target *device.Listed, path string, report transfer.Reporter) error {
	return SendFileAs(ctx, ident, target, path, filepath.Base(path), report)
}

func SendFileAs(ctx context.Context, ident *device.Identity, target *device.Listed, path, offeredName string, report transfer.Reporter) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("directories are not supported in Phase 1")
	}
	name, err := protocol.SafeFileName(offeredName)
	if err != nil {
		return err
	}
	c, err := DialTarget(ctx, ident, target)
	if err != nil {
		return err
	}
	defer c.Close()
	offer := protocol.Offer{
		ID:   newID(),
		Kind: protocol.KindFile,
		Name: name,
		MIME: mimeFromName(name),
		Size: st.Size(),
	}
	return transfer.SendFile(ctx, c, path, offer, report)
}

func mimeFromName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}

func SendClipboard(ctx context.Context, ident *device.Identity, target *device.Listed, it *clipboard.Item, report transfer.Reporter) error {
	c, err := DialTarget(ctx, ident, target)
	if err != nil {
		return err
	}
	defer c.Close()
	offer := protocol.Offer{
		ID:   newID(),
		Kind: protocol.KindClipboard,
		Name: it.Filename,
		MIME: it.MIME,
	}
	var body []byte
	switch it.Kind {
	case history.KindImage:
		offer.ClipFormat = protocol.ClipImage
		body = it.Data
	case history.KindFile:
		offer.ClipFormat = protocol.ClipFile
		body = it.Data
	default:
		offer.ClipFormat = protocol.ClipText
		body = []byte(it.Text)
		if offer.Name == "" {
			offer.Name = "clipboard.txt"
		}
		offer.MIME = "text/plain"
	}
	if offer.Name == "" {
		offer.Name = "clipboard.bin"
	}
	if _, err := protocol.SafeFileName(offer.Name); err != nil {
		offer.Name = "clipboard.bin"
	}
	offer.Size = int64(len(body))
	return transfer.SendBytes(ctx, c, bytes.NewReader(body), offer, report)
}
