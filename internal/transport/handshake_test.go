package transport

import (
	"crypto/ed25519"
	"io"
	"net"
	"testing"
	"time"

	"beam/internal/crypto"
	"beam/internal/device"
	"beam/internal/protocol"
)

func testIdent(t *testing.T, name string) *device.Identity {
	t.Helper()
	pub, priv, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return &device.Identity{
		Config: device.Config{
			DeviceID:   crypto.DeviceID(pub),
			Name:       name,
			Type:       device.TypeMac,
			ListenPort: 47821,
		},
		PrivateKey: priv,
		PublicKey:  pub,
	}
}

func TestPairedHandshakeAndRejectUntrusted(t *testing.T) {
	a := testIdent(t, "MacBook")
	b := testIdent(t, "Windows-PC")
	peerB := &device.Peer{
		ID:        b.Config.DeviceID,
		Name:      b.Config.Name,
		Type:      b.Config.Type,
		PublicKey: crypto.EncodePublicKey(ed25519.PublicKey(b.PublicKey)),
	}
	peerA := &device.Peer{
		ID:        a.Config.DeviceID,
		Name:      a.Config.Name,
		Type:      a.Config.Type,
		PublicKey: crypto.EncodePublicKey(ed25519.PublicKey(a.PublicKey)),
	}
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	errc := make(chan error, 2)
	var srv *Conn
	go func() {
		var err error
		srv, err = HandshakeResponder(c2, b, protocol.ModeData, "", func(id string) (*device.Peer, error) {
			if id == peerA.ID {
				return peerA, nil
			}
			return nil, io.EOF
		})
		errc <- err
	}()
	cli, err := HandshakeInitiator(c1, a, protocol.ModeData, "", peerB)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	defer srv.Close()

	sendErr := make(chan error, 1)
	go func() {
		sendErr <- cli.Send(protocol.MsgPing, []byte("hi"))
	}()
	typ, payload, err := srv.Receive()
	if err != nil || typ != protocol.MsgPing || string(payload) != "hi" {
		t.Fatalf("msg %d %q %v", typ, payload, err)
	}
	if err := <-sendErr; err != nil {
		t.Fatal(err)
	}
}

func TestPairingHandshakeWrongCode(t *testing.T) {
	a := testIdent(t, "A")
	b := testIdent(t, "B")
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	errc := make(chan error, 1)
	go func() {
		_, err := HandshakeResponder(c2, b, protocol.ModePair, "111111", nil)
		_ = c2.Close()
		errc <- err
	}()
	_, err := HandshakeInitiator(c1, a, protocol.ModePair, "000000", nil)
	srvErr := <-errc
	if err == nil && srvErr == nil {
		t.Fatal("wrong pairing code should fail")
	}
}

func TestPairingHandshakeOK(t *testing.T) {
	a := testIdent(t, "A")
	b := testIdent(t, "B")
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	errc := make(chan error, 1)
	go func() {
		_, err := HandshakeResponder(c2, b, protocol.ModePair, "482917", nil)
		errc <- err
	}()
	cli, err := HandshakeInitiator(c1, a, protocol.ModePair, "482917", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	_ = cli
	_ = time.Second
}

func TestUntrustedDataRejected(t *testing.T) {
	a := testIdent(t, "A")
	b := testIdent(t, "B")
	peerB := &device.Peer{
		ID:        b.Config.DeviceID,
		Name:      b.Config.Name,
		PublicKey: crypto.EncodePublicKey(ed25519.PublicKey(b.PublicKey)),
	}
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	errc := make(chan error, 1)
	go func() {
		_, err := HandshakeResponder(c2, b, protocol.ModeData, "", func(id string) (*device.Peer, error) {
			return nil, io.EOF
		})
		_ = c2.Close()
		errc <- err
	}()
	_, _ = HandshakeInitiator(c1, a, protocol.ModeData, "", peerB)
	if err := <-errc; err == nil {
		t.Fatal("untrusted peer should fail on responder")
	}
}
