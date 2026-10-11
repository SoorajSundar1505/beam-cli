package transfer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"beam/internal/protocol"
	"beam/internal/transport"
)

func TestMissingSpansSkipStoredChunks(t *testing.T) {
	have := make([]byte, bitBytes(8))
	bitSet(have, 0)
	bitSet(have, 1)
	bitSet(have, 5)
	got := missingSpans(have, 8)
	want := [][2]uint64{{2, 5}, {6, 8}}
	if len(got) != len(want) {
		t.Fatalf("spans %#v", got)
	}
	for i := range want {
		if got[i].From != want[i][0] || got[i].To != want[i][1] {
			t.Fatalf("span %d = %#v", i, got[i])
		}
	}
	if spanBytes(got, int64(8*ChunkSize), ChunkSize) != int64(5*ChunkSize) {
		t.Fatalf("missing bytes = %d", spanBytes(got, int64(8*ChunkSize), ChunkSize))
	}
}

func TestChunkWriteRejectsDuplicatesAndCorruption(t *testing.T) {
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	payload := bytes.Repeat([]byte{7}, ChunkSize)
	offer := testOffer(t, payload)
	state, err := openIncoming(offer.ID, "peer", "blob.bin", offer)
	if err != nil {
		t.Fatal(err)
	}
	defer state.close()
	if err := state.writeChunk(0, payload[:state.chunkLen(0)]); err != nil {
		t.Fatal(err)
	}
	if err := state.writeChunk(0, payload[:state.chunkLen(0)]); err != nil {
		t.Fatal(err)
	}
	bad := append([]byte(nil), payload[:state.chunkLen(0)]...)
	bad[0] ^= 0xff
	if err := state.writeChunk(0, bad); !errors.Is(err, ErrCorruptChunk) {
		t.Fatalf("duplicate changed chunk: %v", err)
	}
}

func TestResumeAfterDisconnect(t *testing.T) {
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	dir := t.TempDir()
	src := filepath.Join(dir, "blob.bin")
	data := make([]byte, 3*ChunkSize+20)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatal(err)
	}
	offer, err := FileOffer("peer-a", src, "blob.bin")
	if err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	afterChunk = func(index uint64) {
		if index != 0 {
			return
		}
		meta := filepath.Join(os.Getenv("BEAM_DATA_DIR"), "transfers", "in", offer.ID, "meta.json")
		for i := 0; i < 100 && !chunkSaved(meta, 0); i++ {
			time.Sleep(20 * time.Millisecond)
		}
		cancel()
	}
	t.Cleanup(func() { afterChunk = nil })
	cli, srv := pairedConns(t)
	errc := make(chan error, 1)
	go func() {
		errc <- acceptAndReceive(srv, dest)
	}()
	sendErr := SendFile(ctx, cli, src, offer, nil)
	cli.Close()
	if !errors.Is(sendErr, context.Canceled) {
		t.Fatalf("sender: %v", sendErr)
	}
	if err := <-errc; err == nil {
		t.Fatal("receiver finished a partial transfer")
	}

	cli, srv = pairedConns(t)
	defer cli.Close()
	defer srv.Close()
	resumed := false
	errc = make(chan error, 1)
	go func() {
		errc <- acceptAndReceive(srv, dest)
	}()
	if err := SendFile(context.Background(), cli, src, offer, func(p Progress) {
		if p.Resume && p.Bytes > 0 && !p.Done {
			resumed = true
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if !resumed {
		t.Fatal("second attempt did not report resumed progress")
	}
	got, err := os.ReadFile(filepath.Join(dest, "blob.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("resumed file mismatch")
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("BEAM_DATA_DIR"), "transfers", "in", offer.ID)); !os.IsNotExist(err) {
		t.Fatal("resume state was kept after success")
	}
}

func TestCorruptStoredChunkIsSentAgain(t *testing.T) {
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	payload := bytes.Repeat([]byte{9}, ChunkSize*2)
	offer := testOffer(t, payload)
	state, err := openIncoming(offer.ID, "peer", "blob.bin", offer)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.writeChunk(0, payload[:ChunkSize]); err != nil {
		t.Fatal(err)
	}
	if err := state.writeChunk(1, payload[ChunkSize:]); err != nil {
		t.Fatal(err)
	}
	state.close()
	partial := filepath.Join(os.Getenv("BEAM_DATA_DIR"), "transfers", "in", offer.ID, "data")
	stored, err := os.ReadFile(partial)
	if err != nil {
		t.Fatal(err)
	}
	stored[0] ^= 0xff
	if err := os.WriteFile(partial, stored, 0o600); err != nil {
		t.Fatal(err)
	}
	reopened, err := openIncoming(offer.ID, "peer", "blob.bin", offer)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.audit(); err != nil {
		t.Fatal(err)
	}
	if bitGet(reopened.have, 0) || !bitGet(reopened.have, 1) {
		t.Fatal("corrupt chunk was still trusted")
	}
	reopened.close()
}

func TestChangedSourceFileDoesNotResume(t *testing.T) {
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	dir := t.TempDir()
	src := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(src, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := FileOffer("peer-a", src, "blob.bin")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := FileOffer("peer-a", src, "blob.bin"); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("changed file: %v", err)
	}
	again, err := FileOffer("peer-a", src, "blob.bin")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID == first.ID || again.SHA256 == first.SHA256 {
		t.Fatalf("new transfer was not started: %+v", again)
	}
}

func acceptAndReceive(srv *transport.Conn, dest string) error {
	typ, payload, err := srv.Receive()
	if err != nil {
		return err
	}
	if typ != protocol.MsgOffer {
		return errors.New("expected offer")
	}
	var offer protocol.Offer
	if err := json.Unmarshal(payload, &offer); err != nil {
		return err
	}
	_, err = ReceiveToFile(context.Background(), srv, offer, dest, nil)
	return err
}

func chunkSaved(meta string, index int) bool {
	b, err := os.ReadFile(meta)
	if err != nil {
		return false
	}
	var state incoming
	if json.Unmarshal(b, &state) != nil {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(state.Have)
	if err != nil {
		return false
	}
	return bitGet(raw, index)
}

func testOffer(t *testing.T, payload []byte) protocol.Offer {
	t.Helper()
	sum := sha256.Sum256(payload)
	return protocol.Offer{
		ID:        "transfer1",
		Kind:      protocol.KindFile,
		Name:      "blob.bin",
		Size:      int64(len(payload)),
		SHA256:    hex.EncodeToString(sum[:]),
		ChunkSize: ChunkSize,
	}
}
