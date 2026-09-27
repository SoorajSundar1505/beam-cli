package crypto

import (
	"bytes"
	"testing"
)

func TestIdentityAndDeviceID(t *testing.T) {
	pub, priv, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	_ = priv
	id := DeviceID(pub)
	if len(id) != 32 {
		t.Fatalf("id length %d", len(id))
	}
	enc := EncodePublicKey(pub)
	dec, err := DecodePublicKey(enc)
	if err != nil || !bytes.Equal(dec, pub) {
		t.Fatalf("roundtrip key")
	}
}

func TestSessionRoundTrip(t *testing.T) {
	ephA, pubA, err := GenerateEphemeral()
	if err != nil {
		t.Fatal(err)
	}
	ephB, pubB, err := GenerateEphemeral()
	if err != nil {
		t.Fatal(err)
	}
	sa, err := ECDH(ephA, pubB)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := ECDH(ephB, pubA)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sa, sb) {
		t.Fatal("ecdh mismatch")
	}
	tr := Transcript([]byte("a"), []byte("b"))
	sendA, recvA, err := DeriveSessionKeys(sa, tr, "123456", true)
	if err != nil {
		t.Fatal(err)
	}
	sendB, recvB, err := DeriveSessionKeys(sb, tr, "123456", false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sendA, recvB) || !bytes.Equal(recvA, sendB) {
		t.Fatal("key direction mismatch")
	}
	a := NewSession(sendA, recvA)
	b := NewSession(sendB, recvB)
	ct, err := a.Seal([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := b.Open(ct)
	if err != nil || string(pt) != "hello" {
		t.Fatalf("open: %s %v", pt, err)
	}
	_, err = b.Open(ct)
	if err == nil {
		t.Fatal("replay should fail via nonce")
	}
}

func TestWrongPairingCode(t *testing.T) {
	shared := bytes.Repeat([]byte{1}, 32)
	tr := Transcript([]byte("x"))
	a0, a1, _ := DeriveSessionKeys(shared, tr, "111111", true)
	b0, b1, _ := DeriveSessionKeys(shared, tr, "222222", false)
	_ = a1
	_ = b0
	a := NewSession(a0, a1)
	b := NewSession(b0, b1)
	ct, _ := a.Seal([]byte("ok"))
	if _, err := b.Open(ct); err == nil {
		t.Fatal("expected auth failure")
	}
}

func TestRandomDigits(t *testing.T) {
	s, err := RandomDigits(6)
	if err != nil || len(s) != 6 {
		t.Fatalf("%q %v", s, err)
	}
}
