package transfer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"beam/internal/protocol"
	"beam/internal/storage"
	"beam/internal/transport"
)

const MaxAttempts = 3

const keepPartial = 7 * 24 * time.Hour

var (
	ErrSourceChanged = errors.New("source file changed")
	ErrCorruptChunk  = errors.New("corrupted chunk")
)

// Interrupted marks a transfer that stopped before Done. The caller may reconnect.
type Interrupted struct{ Err error }

func (e *Interrupted) Error() string {
	if e.Err == nil {
		return "transfer interrupted"
	}
	return "transfer interrupted: " + e.Err.Error()
}

func (e *Interrupted) Unwrap() error { return e.Err }

func Retryable(err error) bool {
	var interrupted *Interrupted
	return errors.As(err, &interrupted)
}

// afterChunk is a test hook invoked after each chunk is written to the session.
var afterChunk func(index uint64)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

var transferMu sync.Map

func lockID(id string) func() {
	v, _ := transferMu.LoadOrStore(id, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

type outRecord struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	ChunkSize int    `json:"chunk_size"`
}

// FileOffer returns a stable offer for path. The same unchanged file keeps its
// transfer ID so the receiver can ask for only the missing chunks.
func FileOffer(peerID, path, name string) (protocol.Offer, error) {
	st, err := os.Stat(path)
	if err != nil {
		return protocol.Offer{}, err
	}
	sum, err := hashFile(path)
	if err != nil {
		return protocol.Offer{}, err
	}
	rec, recPath, err := loadOut(peerID, path)
	if err != nil {
		return protocol.Offer{}, err
	}
	if rec != nil && rec.SHA256 != sum {
		_ = os.Remove(recPath)
		return protocol.Offer{}, ErrSourceChanged
	}
	if rec == nil {
		rec = &outRecord{
			ID:        newTransferID(),
			Path:      path,
			Name:      name,
			Size:      st.Size(),
			SHA256:    sum,
			ChunkSize: ChunkSize,
		}
		if err := saveJSON(recPath, rec); err != nil {
			return protocol.Offer{}, err
		}
	}
	return protocol.Offer{
		ID:        rec.ID,
		Kind:      protocol.KindFile,
		Name:      name,
		Size:      st.Size(),
		SHA256:    sum,
		ChunkSize: ChunkSize,
	}, nil
}

func sendResume(ctx context.Context, c *transport.Conn, path string, offer protocol.Offer, report Reporter) error {
	if err := c.SendJSON(protocol.MsgOffer, offer); err != nil {
		return &Interrupted{Err: err}
	}
	typ, payload, err := c.Receive()
	if err != nil {
		return &Interrupted{Err: err}
	}
	if typ == protocol.MsgReject {
		var m protocol.IDMsg
		_ = json.Unmarshal(payload, &m)
		if m.Reason == "" {
			m.Reason = "rejected"
		}
		return fmt.Errorf("remote rejected transfer: %s", m.Reason)
	}
	if typ != protocol.MsgAccept {
		return fmt.Errorf("unexpected response %d", typ)
	}
	var accept protocol.IDMsg
	if err := json.Unmarshal(payload, &accept); err != nil {
		return err
	}
	ranges := accept.Missing
	resumed := false
	sentBase := int64(0)
	if accept.Resume {
		missing := spanBytes(ranges, offer.Size, ChunkSize)
		sentBase = offer.Size - missing
		resumed = sentBase > 0
	} else {
		ranges = []protocol.Span{{From: 0, To: uint64(chunkCount(offer.Size))}}
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	start := time.Now()
	sent := sentBase
	emit := func(done bool, e error) {
		if report != nil {
			report(Progress{
				Name: offer.Name, Total: offer.Size, Bytes: sent,
				Started: start, Done: done, Resume: resumed, Err: e,
			})
		}
	}
	emit(false, nil)
	buf := make([]byte, ChunkSize)
	for _, sp := range ranges {
		for index := sp.From; index < sp.To; index++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			offset := int64(index) * int64(ChunkSize)
			n := ChunkSize
			if remain := offer.Size - offset; int64(n) > remain {
				n = int(remain)
			}
			if _, err := f.ReadAt(buf[:n], offset); err != nil && err != io.EOF {
				return err
			}
			if err := c.Send(protocol.MsgData, protocol.EncodeData(index, buf[:n])); err != nil {
				return &Interrupted{Err: err}
			}
			if afterChunk != nil {
				afterChunk(index)
			}
			sent += int64(n)
			emit(false, nil)
		}
	}
	if err := c.SendJSON(protocol.MsgDone, protocol.IDMsg{ID: offer.ID, SHA256: offer.SHA256}); err != nil {
		return &Interrupted{Err: err}
	}
	_ = clearOut(offer.ID)
	emit(true, nil)
	return nil
}

func receiveResume(ctx context.Context, c *transport.Conn, offer protocol.Offer, destDir, peer string, report Reporter) (string, error) {
	name, err := protocol.SafeFileName(offer.Name)
	if err != nil {
		_ = c.SendJSON(protocol.MsgReject, protocol.IDMsg{ID: offer.ID, Reason: "invalid filename"})
		return "", err
	}
	id, err := safeTransferID(offer.ID)
	if err != nil {
		_ = c.SendJSON(protocol.MsgReject, protocol.IDMsg{ID: offer.ID, Reason: "invalid transfer id"})
		return "", err
	}
	if offer.Size < 0 {
		_ = c.SendJSON(protocol.MsgReject, protocol.IDMsg{ID: offer.ID, Reason: "invalid size"})
		return "", fmt.Errorf("invalid size")
	}
	unlock := lockID(id)
	defer unlock()
	_ = sweepExpired()

	state, err := openIncoming(id, peer, name, offer)
	if err != nil {
		reason := "cannot resume"
		if errors.Is(err, ErrSourceChanged) {
			reason = "source file changed"
		}
		_ = c.SendJSON(protocol.MsgReject, protocol.IDMsg{ID: offer.ID, Reason: reason})
		return "", err
	}
	if err := state.audit(); err != nil {
		_ = c.SendJSON(protocol.MsgReject, protocol.IDMsg{ID: offer.ID, Reason: "corrupted chunk"})
		return "", err
	}
	missing := missingSpans(state.have, state.count())
	if err := c.SendJSON(protocol.MsgAccept, protocol.IDMsg{ID: offer.ID, Resume: true, Missing: missing}); err != nil {
		return "", err
	}

	start := time.Now()
	emit := func() {
		if report == nil {
			return
		}
		report(Progress{
			Name: name, Total: offer.Size, Bytes: state.receivedBytes(),
			Started: start, Resume: state.receivedBytes() > 0 && state.receivedBytes() < offer.Size,
		})
	}
	emit()

	for state.receivedBytes() < offer.Size || offer.Size == 0 && !state.doneEmpty {
		if offer.Size == 0 {
			break
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		typ, payload, err := c.Receive()
		if err != nil {
			return "", err
		}
		switch typ {
		case protocol.MsgCancel:
			return "", fmt.Errorf("transfer cancelled")
		case protocol.MsgDone:
			return finishIncoming(state, payload, destDir, name, report, start)
		case protocol.MsgData:
			seq, chunk, err := protocol.DecodeData(payload)
			if err != nil {
				return "", err
			}
			if err := state.writeChunk(int(seq), chunk); err != nil {
				return "", err
			}
			emit()
		default:
			return "", fmt.Errorf("unexpected message %d", typ)
		}
	}
	typ, payload, err := c.Receive()
	if err != nil {
		return "", err
	}
	if typ == protocol.MsgCancel {
		return "", fmt.Errorf("transfer cancelled")
	}
	if typ != protocol.MsgDone {
		return "", fmt.Errorf("unexpected message %d", typ)
	}
	return finishIncoming(state, payload, destDir, name, report, start)
}

type incoming struct {
	dir       string
	ID        string    `json:"id"`
	Peer      string    `json:"peer"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256"`
	ChunkSize int       `json:"chunk_size"`
	Have      string    `json:"have"`
	Updated   time.Time `json:"updated"`
	have      []byte
	data      *os.File
	sums      *os.File
	doneEmpty bool
}

func openIncoming(id, peer, name string, offer protocol.Offer) (*incoming, error) {
	dir, err := incomingDir(id)
	if err != nil {
		return nil, err
	}
	state := &incoming{dir: dir, ID: id, ChunkSize: ChunkSize}
	meta := filepath.Join(dir, "meta.json")
	if b, err := os.ReadFile(meta); err == nil {
		if err := json.Unmarshal(b, state); err != nil {
			return nil, err
		}
		state.dir = dir
		if state.SHA256 != offer.SHA256 || state.Size != offer.Size {
			_ = os.RemoveAll(dir)
			return nil, ErrSourceChanged
		}
		raw, err := base64.StdEncoding.DecodeString(state.Have)
		if err != nil {
			return nil, err
		}
		state.have = raw
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	n := chunkCount(offer.Size)
	if len(state.have) < bitBytes(n) {
		grown := make([]byte, bitBytes(n))
		copy(grown, state.have)
		state.have = grown
	}
	data, err := os.OpenFile(filepath.Join(dir, "data"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := data.Truncate(offer.Size); err != nil {
		data.Close()
		return nil, err
	}
	sums, err := os.OpenFile(filepath.Join(dir, "sums"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		data.Close()
		return nil, err
	}
	if err := sums.Truncate(int64(n) * sha256.Size); err != nil {
		data.Close()
		sums.Close()
		return nil, err
	}
	state.data = data
	state.sums = sums
	state.ID = id
	state.Peer = peer
	state.Name = name
	state.Size = offer.Size
	state.SHA256 = offer.SHA256
	state.ChunkSize = ChunkSize
	if offer.Size == 0 {
		state.doneEmpty = true
	}
	if err := state.save(); err != nil {
		state.close()
		return nil, err
	}
	return state, nil
}

func (s *incoming) count() int { return chunkCount(s.Size) }

func (s *incoming) close() {
	if s.data != nil {
		_ = s.data.Close()
	}
	if s.sums != nil {
		_ = s.sums.Close()
	}
}

func (s *incoming) audit() error {
	buf := make([]byte, ChunkSize)
	stored := make([]byte, sha256.Size)
	for i := 0; i < s.count(); i++ {
		if !bitGet(s.have, i) {
			continue
		}
		n := s.chunkLen(i)
		if _, err := s.data.ReadAt(buf[:n], int64(i)*int64(ChunkSize)); err != nil && err != io.EOF {
			return err
		}
		if _, err := s.sums.ReadAt(stored, int64(i)*sha256.Size); err != nil && err != io.EOF {
			return err
		}
		sum := sha256.Sum256(buf[:n])
		if !equalBytes(sum[:], stored) {
			bitClear(s.have, i)
		}
	}
	return s.save()
}

func (s *incoming) writeChunk(index int, chunk []byte) error {
	if index < 0 || index >= s.count() {
		return ErrCorruptChunk
	}
	if len(chunk) != s.chunkLen(index) {
		return ErrCorruptChunk
	}
	sum := sha256.Sum256(chunk)
	if bitGet(s.have, index) {
		stored := make([]byte, sha256.Size)
		_, err := s.sums.ReadAt(stored, int64(index)*sha256.Size)
		if err == nil && equalBytes(sum[:], stored) {
			return nil
		}
		return ErrCorruptChunk
	}
	if _, err := s.data.WriteAt(chunk, int64(index)*int64(ChunkSize)); err != nil {
		return err
	}
	if _, err := s.sums.WriteAt(sum[:], int64(index)*sha256.Size); err != nil {
		return err
	}
	bitSet(s.have, index)
	return s.save()
}

func (s *incoming) chunkLen(index int) int {
	offset := int64(index) * int64(ChunkSize)
	n := ChunkSize
	if remain := s.Size - offset; int64(n) > remain {
		n = int(remain)
	}
	return n
}

func (s *incoming) receivedBytes() int64 {
	var n int64
	for i := 0; i < s.count(); i++ {
		if bitGet(s.have, i) {
			n += int64(s.chunkLen(i))
		}
	}
	return n
}

func (s *incoming) save() error {
	s.Have = base64.StdEncoding.EncodeToString(s.have)
	s.Updated = time.Now().UTC()
	return saveJSON(filepath.Join(s.dir, "meta.json"), s)
}

func finishIncoming(s *incoming, payload []byte, destDir, name string, report Reporter, start time.Time) (string, error) {
	defer s.close()
	var done protocol.IDMsg
	if err := json.Unmarshal(payload, &done); err != nil {
		return "", err
	}
	if done.SHA256 != s.SHA256 {
		_ = os.RemoveAll(s.dir)
		return "", fmt.Errorf("checksum mismatch")
	}
	if s.receivedBytes() != s.Size {
		return "", fmt.Errorf("incomplete transfer")
	}
	dataPath := s.data.Name()
	if err := s.data.Close(); err != nil {
		return "", err
	}
	s.data = nil
	if err := s.sums.Close(); err != nil {
		return "", err
	}
	s.sums = nil
	sum, err := hashFile(dataPath)
	if err != nil {
		return "", err
	}
	if sum != s.SHA256 {
		_ = os.RemoveAll(s.dir)
		return "", fmt.Errorf("checksum mismatch")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	final := uniquePath(filepath.Join(destDir, name))
	if err := copyToPlace(dataPath, final); err != nil {
		_ = os.Remove(final)
		return "", err
	}
	_ = os.RemoveAll(s.dir)
	if report != nil {
		report(Progress{Name: name, Total: s.Size, Bytes: s.Size, Started: start, Done: true, Resume: true})
	}
	return final, nil
}

func copyToPlace(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func chunkCount(size int64) int {
	if size <= 0 {
		return 0
	}
	return int((size + int64(ChunkSize) - 1) / int64(ChunkSize))
}

func spanBytes(spans []protocol.Span, size int64, chunk int) int64 {
	var n int64
	count := chunkCount(size)
	for _, sp := range spans {
		for i := sp.From; i < sp.To && int(i) < count; i++ {
			length := chunk
			offset := int64(i) * int64(chunk)
			if remain := size - offset; int64(length) > remain {
				length = int(remain)
			}
			n += int64(length)
		}
	}
	return n
}

func missingSpans(have []byte, count int) []protocol.Span {
	var out []protocol.Span
	var start = -1
	flush := func(end int) {
		if start >= 0 {
			out = append(out, protocol.Span{From: uint64(start), To: uint64(end)})
			start = -1
		}
	}
	for i := 0; i < count; i++ {
		if bitGet(have, i) {
			flush(i)
			continue
		}
		if start < 0 {
			start = i
		}
	}
	flush(count)
	return out
}

func bitBytes(n int) int { return (n + 7) / 8 }

func bitGet(b []byte, i int) bool {
	if i < 0 || i/8 >= len(b) {
		return false
	}
	return b[i/8]&(1<<uint(i%8)) != 0
}

func bitSet(b []byte, i int) {
	if i >= 0 && i/8 < len(b) {
		b[i/8] |= 1 << uint(i%8)
	}
}

func bitClear(b []byte, i int) {
	if i >= 0 && i/8 < len(b) {
		b[i/8] &^= 1 << uint(i%8)
	}
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func newTransferID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		sum := sha256.Sum256([]byte(time.Now().Format(time.RFC3339Nano)))
		copy(b[:], sum[:])
	}
	return hex.EncodeToString(b[:])
}

func safeTransferID(id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", fmt.Errorf("invalid transfer id")
	}
	return id, nil
}

func incomingDir(id string) (string, error) {
	root, err := transferRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "in", id), nil
}

func loadOut(peerID, path string) (*outRecord, string, error) {
	root, err := transferRoot()
	if err != nil {
		return nil, "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	sum := sha256.Sum256([]byte(peerID + "\x00" + abs))
	dir := filepath.Join(root, "out", safePeer(peerID))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, "", err
	}
	recPath := filepath.Join(dir, hex.EncodeToString(sum[:16])+".json")
	b, err := os.ReadFile(recPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, recPath, nil
		}
		return nil, recPath, err
	}
	var rec outRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		return nil, recPath, err
	}
	return &rec, recPath, nil
}

func clearOut(id string) error {
	root, err := transferRoot()
	if err != nil {
		return err
	}
	out := filepath.Join(root, "out")
	if _, err := os.Stat(out); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(out, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var rec outRecord
		if json.Unmarshal(b, &rec) == nil && rec.ID == id {
			_ = os.Remove(path)
		}
		return nil
	})
}

func sweepExpired() error {
	root, err := transferRoot()
	if err != nil {
		return err
	}
	in := filepath.Join(root, "in")
	entries, err := os.ReadDir(in)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cutoff := time.Now().Add(-keepPartial)
	for _, entry := range entries {
		meta := filepath.Join(in, entry.Name(), "meta.json")
		var state incoming
		b, err := os.ReadFile(meta)
		if err != nil || json.Unmarshal(b, &state) != nil {
			continue
		}
		if !state.Updated.IsZero() && state.Updated.Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(in, entry.Name()))
		}
	}
	return nil
}

func transferRoot() (string, error) {
	dir, err := storage.DataDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(dir, "transfers")
	return root, os.MkdirAll(root, 0o700)
}

func safePeer(id string) string {
	if idPattern.MatchString(id) {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:8])
}

func saveJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
