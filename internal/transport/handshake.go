package transport

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	bcrcrypto "beam/internal/crypto"
	"beam/internal/device"
	"beam/internal/protocol"
)

type Conn struct {
	raw       net.Conn
	sess      *bcrcrypto.Session
	Local     protocol.Hello
	Remote    protocol.Hello
	Initiator bool
}

func (c *Conn) Close() error {
	return c.raw.Close()
}

func (c *Conn) SetDeadline(t time.Time) error {
	return c.raw.SetDeadline(t)
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.raw.RemoteAddr()
}

func HandshakeInitiator(raw net.Conn, ident *device.Identity, mode protocol.Mode, pairingCode string, trusted *device.Peer) (*Conn, error) {
	return handshake(raw, ident, mode, pairingCode, trusted, true)
}

func HandshakeResponder(raw net.Conn, ident *device.Identity, expectedMode protocol.Mode, pairingCode string, lookup func(id string) (*device.Peer, error)) (*Conn, error) {
	_ = expectedMode
	return handshakeResponder(raw, ident, pairingCode, lookup)
}

func handshake(raw net.Conn, ident *device.Identity, mode protocol.Mode, pairingCode string, trusted *device.Peer, initiator bool) (*Conn, error) {
	_ = raw.SetDeadline(time.Now().Add(30 * time.Second))
	eph, ephPub, err := bcrcrypto.GenerateEphemeral()
	if err != nil {
		return nil, err
	}
	hello := protocol.Hello{
		Version:   protocol.Version,
		Mode:      mode,
		DeviceID:  ident.Config.DeviceID,
		Name:      ident.Config.Name,
		Type:      string(ident.Config.Type),
		PublicKey: ident.PublicKey,
		EphPub:    ephPub,
		Timestamp: time.Now().Unix(),
	}
	hello.Sign(ed25519.PrivateKey(ident.PrivateKey))
	localBytes, err := json.Marshal(hello)
	if err != nil {
		return nil, err
	}
	if err := writeClear(raw, localBytes); err != nil {
		return nil, err
	}
	remoteBytes, err := readClear(raw)
	if err != nil {
		return nil, err
	}
	var remote protocol.Hello
	if err := json.Unmarshal(remoteBytes, &remote); err != nil {
		return nil, err
	}
	if err := remote.Verify(); err != nil {
		return nil, err
	}
	if remote.DeviceID == ident.Config.DeviceID {
		return nil, errors.New("refusing to connect to this device")
	}
	if mode == protocol.ModeData {
		if trusted == nil {
			return nil, errors.New("peer is not paired")
		}
		if trusted.ID != remote.DeviceID {
			return nil, errors.New("peer identity mismatch")
		}
		want, err := bcrcrypto.DecodePublicKey(trusted.PublicKey)
		if err != nil {
			return nil, err
		}
		if !ed25519.PublicKey(remote.PublicKey).Equal(want) {
			return nil, errors.New("peer public key mismatch")
		}
	}
	shared, err := bcrcrypto.ECDH(eph, remote.EphPub)
	if err != nil {
		return nil, err
	}
	transcript := bcrcrypto.Transcript(localBytes, remoteBytes)
	send, recv, err := bcrcrypto.DeriveSessionKeys(shared, transcript, pairingCode, true)
	if err != nil {
		return nil, err
	}
	sess := bcrcrypto.NewSession(send, recv)
	c := &Conn{raw: raw, sess: sess, Local: hello, Remote: remote, Initiator: initiator}
	if err := c.Send(protocol.MsgPing, nil); err != nil {
		return nil, fmt.Errorf("confirm send: %w", err)
	}
	typ, _, err := c.Receive()
	if err != nil {
		return nil, fmt.Errorf("pairing/auth failed (wrong code or untrusted peer): %w", err)
	}
	if typ != protocol.MsgPong && typ != protocol.MsgPing {
		return nil, errors.New("unexpected confirm message")
	}
	_ = raw.SetDeadline(time.Time{})
	return c, nil
}

func handshakeResponder(raw net.Conn, ident *device.Identity, pairingCode string, lookup func(id string) (*device.Peer, error)) (*Conn, error) {
	_ = raw.SetDeadline(time.Now().Add(30 * time.Second))
	remoteBytes, err := readClear(raw)
	if err != nil {
		return nil, err
	}
	var remote protocol.Hello
	if err := json.Unmarshal(remoteBytes, &remote); err != nil {
		return nil, err
	}
	if err := remote.Verify(); err != nil {
		return nil, err
	}
	if remote.DeviceID == ident.Config.DeviceID {
		return nil, errors.New("refusing to connect to this device")
	}

	mode := remote.Mode
	if err := pairingAllowed(string(mode), pairingCode); err != nil {
		return nil, err
	}
	if mode == protocol.ModeData {
		if lookup == nil {
			return nil, errors.New("peer is not paired")
		}
		p, err := lookup(remote.DeviceID)
		if err != nil {
			return nil, err
		}
		want, err := bcrcrypto.DecodePublicKey(p.PublicKey)
		if err != nil {
			return nil, err
		}
		if !ed25519.PublicKey(remote.PublicKey).Equal(want) {
			return nil, errors.New("peer public key mismatch")
		}
	} else if mode != protocol.ModePair {
		return nil, errors.New("unknown handshake mode")
	}

	eph, ephPub, err := bcrcrypto.GenerateEphemeral()
	if err != nil {
		return nil, err
	}
	hello := protocol.Hello{
		Version:   protocol.Version,
		Mode:      mode,
		DeviceID:  ident.Config.DeviceID,
		Name:      ident.Config.Name,
		Type:      string(ident.Config.Type),
		PublicKey: ident.PublicKey,
		EphPub:    ephPub,
		Timestamp: time.Now().Unix(),
	}
	hello.Sign(ed25519.PrivateKey(ident.PrivateKey))
	localBytes, err := json.Marshal(hello)
	if err != nil {
		return nil, err
	}
	if err := writeClear(raw, localBytes); err != nil {
		return nil, err
	}
	shared, err := bcrcrypto.ECDH(eph, remote.EphPub)
	if err != nil {
		return nil, err
	}
	transcript := bcrcrypto.Transcript(remoteBytes, localBytes)
	code := pairingCode
	if mode != protocol.ModePair {
		code = ""
	}
	send, recv, err := bcrcrypto.DeriveSessionKeys(shared, transcript, code, false)
	if err != nil {
		return nil, err
	}
	sess := bcrcrypto.NewSession(send, recv)
	c := &Conn{raw: raw, sess: sess, Local: hello, Remote: remote, Initiator: false}
	typ, _, err := c.Receive()
	if err != nil {
		return nil, fmt.Errorf("pairing/auth failed (wrong code or untrusted peer): %w", err)
	}
	if typ != protocol.MsgPing {
		return nil, errors.New("unexpected confirm message")
	}
	if err := c.Send(protocol.MsgPong, nil); err != nil {
		return nil, err
	}
	_ = raw.SetDeadline(time.Time{})
	return c, nil
}

func writeClear(w io.Writer, p []byte) error {
	if len(p) > 1<<20 {
		return errors.New("hello too large")
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(p)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(p)
	return err
}

func readClear(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 || n > 1<<20 {
		return nil, errors.New("invalid hello length")
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (c *Conn) Send(t protocol.MsgType, payload []byte) error {
	plain := append([]byte{byte(t)}, payload...)
	ct, err := c.sess.Seal(plain)
	if err != nil {
		return err
	}
	if len(ct) > protocol.MaxFrame+chachaOverhead {
		return errors.New("frame too large")
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(ct)))
	if _, err := c.raw.Write(hdr[:]); err != nil {
		return err
	}
	_, err = c.raw.Write(ct)
	return err
}

const chachaOverhead = 16

func (c *Conn) Receive() (protocol.MsgType, []byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(c.raw, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 || int(n) > protocol.MaxFrame+chachaOverhead {
		return 0, nil, errors.New("invalid frame length")
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(c.raw, buf); err != nil {
		return 0, nil, err
	}
	plain, err := c.sess.Open(buf)
	if err != nil {
		return 0, nil, err
	}
	if len(plain) < 1 {
		return 0, nil, errors.New("empty frame")
	}
	return protocol.MsgType(plain[0]), plain[1:], nil
}

func (c *Conn) SendJSON(t protocol.MsgType, v any) error {
	return c.Send(t, protocol.Marshal(v))
}
