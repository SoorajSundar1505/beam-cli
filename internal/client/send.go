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

func DialTarget(ctx context.Context, ident *device.Identity, target *device.Listed) (*transport.Conn, error) {
	if target.Self {
		return nil, fmt.Errorf("cannot send to this device")
	}
	if target.Peer == nil {
		return nil, fmt.Errorf("device %s is not paired; run beam pair", target.Name)
	}
	addr := target.Addr
	if addr == "" || target.Status == "offline" {
		r, err := discovery.FindID(ctx, target.ID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", target.Name, ErrOffline)
		}
		addr = r.Addr
	}
	return server.DialPeer(ctx, ident, target.Peer, addr)
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
