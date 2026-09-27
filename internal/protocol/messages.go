package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	Version     uint8 = 1
	MaxFrame    int   = 64*1024 + 16
	MaxData     int   = 64 * 1024
	ServiceType       = "_beam._tcp"
	Domain            = "local."
)

type Mode string

const (
	ModePair Mode = "pair"
	ModeData Mode = "data"
)

type MsgType uint8

const (
	MsgOffer  MsgType = 1
	MsgAccept MsgType = 2
	MsgReject MsgType = 3
	MsgData   MsgType = 4
	MsgDone   MsgType = 5
	MsgCancel MsgType = 6
	MsgError  MsgType = 7
	MsgPing   MsgType = 8
	MsgPong   MsgType = 9
)

type Kind string

const (
	KindFile      Kind = "file"
	KindClipboard Kind = "clipboard"
)

type ClipFormat string

const (
	ClipText  ClipFormat = "text"
	ClipImage ClipFormat = "image"
	ClipFile  ClipFormat = "file"
)

type Hello struct {
	Version   uint8  `json:"version"`
	Mode      Mode   `json:"mode"`
	DeviceID  string `json:"device_id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	PublicKey []byte `json:"public_key"`
	EphPub    []byte `json:"eph_pub"`
	Timestamp int64  `json:"timestamp"`
	Signature []byte `json:"signature"`
}

func (h Hello) SignedPayload() []byte {
	copy := h
	copy.Signature = nil
	b, _ := json.Marshal(copy)
	return b
}

func (h *Hello) Sign(priv ed25519.PrivateKey) {
	h.Signature = ed25519.Sign(priv, h.SignedPayload())
}

func (h Hello) Verify() error {
	if h.Version != Version {
		return fmt.Errorf("unsupported protocol version %d", h.Version)
	}
	if len(h.PublicKey) != ed25519.PublicKeySize {
		return errors.New("invalid identity public key")
	}
	if h.DeviceID != deviceID(ed25519.PublicKey(h.PublicKey)) {
		return errors.New("device id does not match identity public key")
	}
	if !ed25519.Verify(ed25519.PublicKey(h.PublicKey), h.SignedPayload(), h.Signature) {
		return errors.New("invalid hello signature")
	}
	now := time.Now().Unix()
	if h.Timestamp < now-300 || h.Timestamp > now+300 {
		return errors.New("hello timestamp out of range")
	}
	return nil
}

func deviceID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return fmt.Sprintf("%x", sum[:16])
}

type Offer struct {
	ID         string     `json:"id"`
	Kind       Kind       `json:"kind"`
	ClipFormat ClipFormat `json:"clip_format,omitempty"`
	Name       string     `json:"name"`
	MIME       string     `json:"mime,omitempty"`
	Size       int64      `json:"size"`
}

type IDMsg struct {
	ID     string `json:"id"`
	Reason string `json:"reason,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

type ErrorMsg struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Marshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func EncodeData(seq uint64, chunk []byte) []byte {
	buf := make([]byte, 8+len(chunk))
	binary.BigEndian.PutUint64(buf[:8], seq)
	copy(buf[8:], chunk)
	return buf
}

func DecodeData(p []byte) (seq uint64, chunk []byte, err error) {
	if len(p) < 8 {
		return 0, nil, errors.New("short data frame")
	}
	seq = binary.BigEndian.Uint64(p[:8])
	chunk = p[8:]
	return seq, chunk, nil
}

func JoinHelloBytes(a, b []byte) []byte {
	return bytes.Join([][]byte{a, b}, []byte{0})
}
