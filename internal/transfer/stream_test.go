package transfer

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"beam/internal/crypto"
	"beam/internal/device"
	"beam/internal/protocol"
	"beam/internal/transport"
)

func idents(t *testing.T) (a, b *device.Identity, pa, pb *device.Peer) {
	t.Helper()
	makeID := func(name string) *device.Identity {
		pub, priv, err := crypto.GenerateIdentity()
		if err != nil {
			t.Fatal(err)
		}
		return &device.Identity{
			Config:     device.Config{DeviceID: crypto.DeviceID(pub), Name: name, Type: device.TypeMac},
			PrivateKey: priv,
			PublicKey:  pub,
		}
	}
	a, b = makeID("A"), makeID("B")
	pa = &device.Peer{ID: a.Config.DeviceID, Name: a.Config.Name, PublicKey: crypto.EncodePublicKey(a.PublicKey)}
	pb = &device.Peer{ID: b.Config.DeviceID, Name: b.Config.Name, PublicKey: crypto.EncodePublicKey(b.PublicKey)}
	return
}

func pairedConns(t *testing.T) (*transport.Conn, *transport.Conn) {
	a, b, pa, pb := idents(t)
	c1, c2 := net.Pipe()
	errc := make(chan error, 1)
	var srv *transport.Conn
	go func() {
		var err error
		srv, err = transport.HandshakeResponder(c2, b, protocol.ModeData, "", func(id string) (*device.Peer, error) {
			return pa, nil
		})
		errc <- err
	}()
	cli, err := transport.HandshakeInitiator(c1, a, protocol.ModeData, "", pb)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	return cli, srv
}

func TestFileTransferStreamingAndChecksum(t *testing.T) {
	cli, srv := pairedConns(t)
	defer cli.Close()
	defer srv.Close()

	dir := t.TempDir()
	src := filepath.Join(dir, "blob.bin")
	data := make([]byte, 256*1024)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	errc := make(chan error, 1)
	go func() {
		typ, payload, err := srv.Receive()
		if err != nil {
			errc <- err
			return
		}
		if typ != protocol.MsgOffer {
			errc <- errors.New("expected offer")
			return
		}
		var offer protocol.Offer
		if err := jsonUnmarshal(payload, &offer); err != nil {
			errc <- err
			return
		}
		_, err = ReceiveToFile(context.Background(), srv, offer, dest, nil)
		errc <- err
	}()
	offer := protocol.Offer{ID: "1", Kind: protocol.KindFile, Name: "blob.bin", Size: int64(len(data))}
	if err := SendFile(context.Background(), cli, src, offer, nil); err != nil {
		t.Fatal(err)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "blob.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("content mismatch")
	}
}

func TestCorruptChecksum(t *testing.T) {
	cli, srv := pairedConns(t)
	defer cli.Close()
	defer srv.Close()
	dest := t.TempDir()
	errc := make(chan error, 1)
	go func() {
		typ, payload, err := srv.Receive()
		if err != nil {
			errc <- err
			return
		}
		var offer protocol.Offer
		_ = jsonUnmarshal(payload, &offer)
		_ = typ
		_, err = ReceiveToFile(context.Background(), srv, offer, dest, nil)
		errc <- err
	}()
	offer := protocol.Offer{ID: "1", Kind: protocol.KindFile, Name: "x.txt", Size: 4}
	if err := cli.SendJSON(protocol.MsgOffer, offer); err != nil {
		t.Fatal(err)
	}
	typ, _, err := cli.Receive()
	if err != nil || typ != protocol.MsgAccept {
		t.Fatalf("accept %v %v", typ, err)
	}
	if err := cli.Send(protocol.MsgData, protocol.EncodeData(0, []byte("abcd"))); err != nil {
		t.Fatal(err)
	}
	if err := cli.SendJSON(protocol.MsgDone, protocol.IDMsg{ID: "1", SHA256: "deadbeef"}); err != nil {
		t.Fatal(err)
	}
	err = <-errc
	if err == nil {
		t.Fatal("expected checksum mismatch")
	}
	matches, _ := filepath.Glob(filepath.Join(dest, "*"))
	if len(matches) != 0 {
		t.Fatalf("partial should be removed: %v", matches)
	}
}

func TestInterruptedTransfer(t *testing.T) {
	cli, srv := pairedConns(t)
	defer srv.Close()
	dest := t.TempDir()
	errc := make(chan error, 1)
	go func() {
		typ, payload, err := srv.Receive()
		if err != nil {
			errc <- err
			return
		}
		var offer protocol.Offer
		_ = jsonUnmarshal(payload, &offer)
		_ = typ
		_, err = ReceiveToFile(context.Background(), srv, offer, dest, nil)
		errc <- err
	}()
	offer := protocol.Offer{ID: "1", Kind: protocol.KindFile, Name: "big.bin", Size: 1 << 20}
	if err := cli.SendJSON(protocol.MsgOffer, offer); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cli.Receive(); err != nil {
		t.Fatal(err)
	}
	if err := cli.Send(protocol.MsgData, protocol.EncodeData(0, bytes.Repeat([]byte("a"), 1024))); err != nil {
		t.Fatal(err)
	}
	_ = cli.Close()
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("expected interrupt error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func TestClipboardBytes(t *testing.T) {
	cli, srv := pairedConns(t)
	defer cli.Close()
	defer srv.Close()
	errc := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		typ, payload, err := srv.Receive()
		if err != nil {
			errCh <- err
			return
		}
		var offer protocol.Offer
		_ = jsonUnmarshal(payload, &offer)
		_ = typ
		b, err := ReceiveToBuffer(context.Background(), srv, offer, 1<<20, nil)
		if err != nil {
			errCh <- err
			return
		}
		errc <- b
	}()
	msg := []byte("hello from Mac")
	offer := protocol.Offer{ID: "c", Kind: protocol.KindClipboard, ClipFormat: protocol.ClipText, Name: "clipboard.txt", Size: int64(len(msg))}
	if err := SendBytes(context.Background(), cli, bytes.NewReader(msg), offer, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case b := <-errc:
		if string(b) != string(msg) {
			t.Fatalf("got %q", b)
		}
	case err := <-errCh:
		t.Fatal(err)
	}
}

func TestCancel(t *testing.T) {
	cli, srv := pairedConns(t)
	defer cli.Close()
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	dest := t.TempDir()
	errc := make(chan error, 1)
	go func() {
		typ, payload, err := srv.Receive()
		if err != nil {
			errc <- err
			return
		}
		var offer protocol.Offer
		_ = jsonUnmarshal(payload, &offer)
		_ = typ
		_, err = ReceiveToFile(context.Background(), srv, offer, dest, nil)
		errc <- err
	}()
	r, w := io.Pipe()
	go func() {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 64*1024))
		cancel()
		_ = w.Close()
	}()
	offer := protocol.Offer{ID: "1", Kind: protocol.KindFile, Name: "x.bin", Size: 10 << 20}
	_ = SendBytes(ctx, cli, r, offer, nil)
	select {
	case <-errc:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
}

func jsonUnmarshal(b []byte, v any) error {
	return unmarshalJSON(b, v)
}
