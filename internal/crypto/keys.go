package crypto

import (
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const IdentityKeySize = ed25519.PrivateKeySize

func GenerateIdentity() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	return pub, priv, err
}

func DeviceID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return fmt.Sprintf("%x", sum[:16])
}

func EncodePublicKey(pub ed25519.PublicKey) string {
	return base64.RawStdEncoding.EncodeToString(pub)
}

func DecodePublicKey(s string) (ed25519.PublicKey, error) {
	b, err := base64.RawStdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(b) != ed25519.PublicKeySize {
		return nil, errors.New("invalid public key length")
	}
	return ed25519.PublicKey(b), nil
}

func SavePrivateKey(path string, priv ed25519.PrivateKey) error {
	if err := os.WriteFile(path, priv, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func LoadPrivateKey(path string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid identity key file")
	}
	return ed25519.PrivateKey(b), nil
}

func PublicFromPrivate(priv ed25519.PrivateKey) ed25519.PublicKey {
	return priv.Public().(ed25519.PublicKey)
}

func GenerateEphemeral() (*ecdh.PrivateKey, []byte, error) {
	curve := ecdh.X25519()
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return priv, priv.PublicKey().Bytes(), nil
}

func ECDH(priv *ecdh.PrivateKey, peerPub []byte) ([]byte, error) {
	curve := ecdh.X25519()
	pub, err := curve.NewPublicKey(peerPub)
	if err != nil {
		return nil, err
	}
	return priv.ECDH(pub)
}

func DeriveSessionKeys(shared, transcript []byte, pairingCode string, initiator bool) (send, recv []byte, err error) {
	info := append([]byte("beam-v1"), transcript...)
	if pairingCode != "" {
		info = append(info, []byte(pairingCode)...)
	}
	buf := make([]byte, 64)
	r := hkdf.New(sha256.New, shared, nil, info)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, nil, err
	}
	k0, k1 := buf[:32], buf[32:]
	if initiator {
		return k0, k1, nil
	}
	return k1, k0, nil
}

func Transcript(parts ...[]byte) []byte {
	h := sha256.New()
	for _, p := range parts {
		var n [4]byte
		binary.BigEndian.PutUint32(n[:], uint32(len(p)))
		h.Write(n[:])
		h.Write(p)
	}
	return h.Sum(nil)
}

type Session struct {
	sendKey []byte
	recvKey []byte
	sendN   uint64
	recvN   uint64
}

func NewSession(send, recv []byte) *Session {
	return &Session{sendKey: send, recvKey: recv}
}

func nonce(n uint64) []byte {
	out := make([]byte, chacha20poly1305.NonceSize)
	binary.BigEndian.PutUint64(out[4:], n)
	return out
}

func (s *Session) Seal(plaintext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(s.sendKey)
	if err != nil {
		return nil, err
	}
	n := s.sendN
	s.sendN++
	return aead.Seal(nil, nonce(n), plaintext, nil), nil
}

func (s *Session) Open(ciphertext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.New(s.recvKey)
	if err != nil {
		return nil, err
	}
	n := s.recvN
	s.recvN++
	return aead.Open(nil, nonce(n), ciphertext, nil)
}

func RandomDigits(n int) (string, error) {
	if n <= 0 {
		return "", errors.New("invalid length")
	}
	const digits = "0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i := range b {
		out[i] = digits[int(b[i])%10]
	}
	return string(out), nil
}
