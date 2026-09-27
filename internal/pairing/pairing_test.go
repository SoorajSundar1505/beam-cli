package pairing

import (
	"crypto/ed25519"
	"testing"

	"beam/internal/crypto"
	"beam/internal/protocol"
)

func TestPeerFromHello(t *testing.T) {
	pub, _, err := crypto.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	h := protocol.Hello{DeviceID: "id1", Name: "iPhone", Type: "ios", PublicKey: pub}
	p := peerFromHello(h)
	if p.ID != "id1" || p.Type != "ios" {
		t.Fatalf("%+v", p)
	}
	if p.PublicKey != crypto.EncodePublicKey(ed25519.PublicKey(pub)) {
		t.Fatal("key")
	}
}
