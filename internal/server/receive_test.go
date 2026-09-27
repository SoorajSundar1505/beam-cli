package server

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"beam/internal/clipboard"
	"beam/internal/crypto"
	"beam/internal/device"
	"beam/internal/history"
	"beam/internal/protocol"
	"beam/internal/transfer"
	"beam/internal/transport"
)

func TestHandleFileAndOfflineName(t *testing.T) {
	t.Setenv("BEAM_CONFIG_DIR", t.TempDir())
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	recvIdent, err := device.Init("Windows-PC")
	if err != nil {
		t.Fatal(err)
	}
	sendPub, sendPriv, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	sendIdent := &device.Identity{
		Config: device.Config{
			DeviceID: crypto.DeviceID(sendPub),
			Name:     "MacBook",
			Type:     device.TypeMac,
		},
		PrivateKey: sendPriv,
		PublicKey:  sendPub,
	}
	if err := device.UpsertPeer(device.Peer{
		ID:        sendIdent.Config.DeviceID,
		Name:      sendIdent.Config.Name,
		Type:      sendIdent.Config.Type,
		PublicKey: crypto.EncodePublicKey(sendIdent.PublicKey),
	}); err != nil {
		t.Fatal(err)
	}

	c1, c2 := net.Pipe()
	dest := t.TempDir()
	var out bytes.Buffer
	errc := make(chan error, 1)
	go func() {
		errc <- HandleConn(c2, Options{
			Ident:     recvIdent,
			Downloads: dest,
			Prompt:    func(string, protocol.Offer) bool { return true },
			Out:       &out,
		})
	}()

	peerRecv := &device.Peer{
		ID:        recvIdent.Config.DeviceID,
		Name:      recvIdent.Config.Name,
		Type:      recvIdent.Config.Type,
		PublicKey: crypto.EncodePublicKey(recvIdent.PublicKey),
	}
	cli, err := transport.HandshakeInitiator(c1, sendIdent, protocol.ModeData, "", peerRecv)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(src, []byte("jpeg-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := transfer.SendFile(context.Background(), cli, src, protocol.Offer{
		ID: "x", Kind: protocol.KindFile, Name: "photo.jpg", Size: 10,
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "photo.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "jpeg-bytes" {
		t.Fatalf("got %q", b)
	}
}

func TestClipboardTransferToPlatform(t *testing.T) {
	t.Setenv("BEAM_CONFIG_DIR", t.TempDir())
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	recvIdent, err := device.Init("Windows-PC")
	if err != nil {
		t.Fatal(err)
	}
	sendPub, sendPriv, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	sendIdent := &device.Identity{
		Config:     device.Config{DeviceID: crypto.DeviceID(sendPub), Name: "MacBook", Type: device.TypeMac},
		PrivateKey: sendPriv,
		PublicKey:  sendPub,
	}
	if err := device.UpsertPeer(device.Peer{
		ID: sendIdent.Config.DeviceID, Name: sendIdent.Config.Name, Type: sendIdent.Config.Type,
		PublicKey: crypto.EncodePublicKey(sendIdent.PublicKey),
	}); err != nil {
		t.Fatal(err)
	}
	st, err := history.OpenPath(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	plat := &memClip{}
	svc := clipboard.New(plat, st)

	c1, c2 := net.Pipe()
	errc := make(chan error, 1)
	go func() {
		errc <- HandleConn(c2, Options{
			Ident:     recvIdent,
			Downloads: t.TempDir(),
			Clip:      svc,
			Prompt:    func(string, protocol.Offer) bool { return true },
			Out:       io.Discard,
		})
	}()
	peerRecv := &device.Peer{
		ID: recvIdent.Config.DeviceID, Name: recvIdent.Config.Name, Type: recvIdent.Config.Type,
		PublicKey: crypto.EncodePublicKey(recvIdent.PublicKey),
	}
	cli, err := transport.HandshakeInitiator(c1, sendIdent, protocol.ModeData, "", peerRecv)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("Hello from Mac")
	if err := transfer.SendBytes(context.Background(), cli, bytes.NewReader(msg), protocol.Offer{
		ID: "c", Kind: protocol.KindClipboard, ClipFormat: protocol.ClipText, Name: "clipboard.txt", Size: int64(len(msg)),
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if plat.cur == nil || plat.cur.Text != "Hello from Mac" {
		t.Fatalf("clipboard %+v", plat.cur)
	}
	_ = time.Second
	_ = strings.TrimSpace
}

type memClip struct{ cur *clipboard.Item }

func (m *memClip) Read() (*clipboard.Item, error) { return m.cur, nil }
func (m *memClip) Write(it *clipboard.Item) error { m.cur = it; return nil }
