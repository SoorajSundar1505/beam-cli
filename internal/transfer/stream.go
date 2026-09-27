package transfer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"beam/internal/protocol"
	"beam/internal/transport"
)

const chunkSize = 64 * 1024

type Progress struct {
	Name    string
	Total   int64
	Bytes   int64
	Started time.Time
	Done    bool
	Err     error
}

func (p Progress) Percent() int {
	if p.Total <= 0 {
		return 0
	}
	n := int(p.Bytes * 100 / p.Total)
	if n > 100 {
		return 100
	}
	return n
}

func (p Progress) Speed() float64 {
	elapsed := time.Since(p.Started).Seconds()
	if elapsed <= 0 {
		return 0
	}
	return float64(p.Bytes) / elapsed
}

func (p Progress) ETA() time.Duration {
	spd := p.Speed()
	if spd <= 0 || p.Bytes >= p.Total {
		return 0
	}
	remain := float64(p.Total-p.Bytes) / spd
	return time.Duration(remain * float64(time.Second))
}

type Reporter func(Progress)

func SendFile(ctx context.Context, c *transport.Conn, path string, offer protocol.Offer, report Reporter) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return sendStream(ctx, c, f, offer, report)
}

func SendBytes(ctx context.Context, c *transport.Conn, r io.Reader, offer protocol.Offer, report Reporter) error {
	return sendStream(ctx, c, r, offer, report)
}

func sendStream(ctx context.Context, c *transport.Conn, r io.Reader, offer protocol.Offer, report Reporter) error {
	if err := c.SendJSON(protocol.MsgOffer, offer); err != nil {
		return err
	}
	typ, payload, err := c.Receive()
	if err != nil {
		return err
	}
	switch typ {
	case protocol.MsgReject:
		var m protocol.IDMsg
		_ = json.Unmarshal(payload, &m)
		if m.Reason == "" {
			m.Reason = "rejected"
		}
		return fmt.Errorf("remote rejected transfer: %s", m.Reason)
	case protocol.MsgAccept:
	default:
		return fmt.Errorf("unexpected response %d", typ)
	}

	h := sha256.New()
	buf := make([]byte, chunkSize)
	var seq uint64
	var sent int64
	start := time.Now()
	emit := func(done bool, e error) {
		if report != nil {
			report(Progress{Name: offer.Name, Total: offer.Size, Bytes: sent, Started: start, Done: done, Err: e})
		}
	}
	emit(false, nil)

	for {
		if err := ctx.Err(); err != nil {
			_ = c.SendJSON(protocol.MsgCancel, protocol.IDMsg{ID: offer.ID})
			return err
		}
		n, readErr := r.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			if err := c.Send(protocol.MsgData, protocol.EncodeData(seq, buf[:n])); err != nil {
				return err
			}
			seq++
			sent += int64(n)
			emit(false, nil)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = c.SendJSON(protocol.MsgCancel, protocol.IDMsg{ID: offer.ID})
			return readErr
		}
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if err := c.SendJSON(protocol.MsgDone, protocol.IDMsg{ID: offer.ID, SHA256: sum}); err != nil {
		return err
	}
	emit(true, nil)
	return nil
}

func ReceiveToFile(ctx context.Context, c *transport.Conn, offer protocol.Offer, destDir string, report Reporter) (string, error) {
	name, err := protocol.SafeFileName(offer.Name)
	if err != nil {
		_ = c.SendJSON(protocol.MsgReject, protocol.IDMsg{ID: offer.ID, Reason: "invalid filename"})
		return "", err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		_ = c.SendJSON(protocol.MsgReject, protocol.IDMsg{ID: offer.ID, Reason: "cannot write destination"})
		return "", err
	}
	final := uniquePath(filepath.Join(destDir, name))
	partial := final + ".beam.partial"
	f, err := os.OpenFile(partial, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		_ = c.SendJSON(protocol.MsgReject, protocol.IDMsg{ID: offer.ID, Reason: "cannot create file"})
		return "", err
	}
	if err := c.SendJSON(protocol.MsgAccept, protocol.IDMsg{ID: offer.ID}); err != nil {
		f.Close()
		_ = os.Remove(partial)
		return "", err
	}
	h := sha256.New()
	var got int64
	var expectSeq uint64
	start := time.Now()
	emit := func() {
		if report != nil {
			report(Progress{Name: name, Total: offer.Size, Bytes: got, Started: start})
		}
	}
	cleanupFail := func() {
		f.Close()
		_ = os.Remove(partial)
	}

	for {
		if err := ctx.Err(); err != nil {
			cleanupFail()
			return "", err
		}
		typ, payload, err := c.Receive()
		if err != nil {
			cleanupFail()
			return "", err
		}
		switch typ {
		case protocol.MsgCancel:
			cleanupFail()
			return "", fmt.Errorf("transfer cancelled")
		case protocol.MsgError:
			cleanupFail()
			var em protocol.ErrorMsg
			_ = json.Unmarshal(payload, &em)
			return "", fmt.Errorf("remote error: %s", em.Message)
		case protocol.MsgData:
			seq, chunk, err := protocol.DecodeData(payload)
			if err != nil {
				cleanupFail()
				return "", err
			}
			if seq != expectSeq {
				cleanupFail()
				return "", fmt.Errorf("interrupted or corrupt transfer: unexpected sequence")
			}
			expectSeq++
			if _, err := f.Write(chunk); err != nil {
				cleanupFail()
				return "", err
			}
			h.Write(chunk)
			got += int64(len(chunk))
			if offer.Size > 0 && got > offer.Size {
				cleanupFail()
				return "", fmt.Errorf("corrupt transfer: size exceeded")
			}
			emit()
		case protocol.MsgDone:
			var done protocol.IDMsg
			if err := json.Unmarshal(payload, &done); err != nil {
				cleanupFail()
				return "", err
			}
			sum := hex.EncodeToString(h.Sum(nil))
			if done.SHA256 != sum {
				cleanupFail()
				return "", fmt.Errorf("checksum mismatch")
			}
			if offer.Size > 0 && got != offer.Size {
				cleanupFail()
				return "", fmt.Errorf("size mismatch")
			}
			if err := f.Close(); err != nil {
				_ = os.Remove(partial)
				return "", err
			}
			if err := os.Rename(partial, final); err != nil {
				_ = os.Remove(partial)
				return "", err
			}
			if report != nil {
				report(Progress{Name: name, Total: got, Bytes: got, Started: start, Done: true})
			}
			return final, nil
		default:
			cleanupFail()
			return "", fmt.Errorf("unexpected message %d", typ)
		}
	}
}

func ReceiveToBuffer(ctx context.Context, c *transport.Conn, offer protocol.Offer, max int64, report Reporter) ([]byte, error) {
	if max > 0 && offer.Size > max {
		_ = c.SendJSON(protocol.MsgReject, protocol.IDMsg{ID: offer.ID, Reason: "clipboard payload too large"})
		return nil, fmt.Errorf("clipboard payload too large")
	}
	if err := c.SendJSON(protocol.MsgAccept, protocol.IDMsg{ID: offer.ID}); err != nil {
		return nil, err
	}
	h := sha256.New()
	var buf []byte
	var expectSeq uint64
	start := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		typ, payload, err := c.Receive()
		if err != nil {
			return nil, err
		}
		switch typ {
		case protocol.MsgCancel:
			return nil, fmt.Errorf("transfer cancelled")
		case protocol.MsgData:
			seq, chunk, err := protocol.DecodeData(payload)
			if err != nil {
				return nil, err
			}
			if seq != expectSeq {
				return nil, fmt.Errorf("interrupted or corrupt transfer: unexpected sequence")
			}
			expectSeq++
			buf = append(buf, chunk...)
			h.Write(chunk)
			if max > 0 && int64(len(buf)) > max {
				return nil, fmt.Errorf("clipboard payload too large")
			}
			if report != nil {
				report(Progress{Name: offer.Name, Total: offer.Size, Bytes: int64(len(buf)), Started: start})
			}
		case protocol.MsgDone:
			var done protocol.IDMsg
			if err := json.Unmarshal(payload, &done); err != nil {
				return nil, err
			}
			sum := hex.EncodeToString(h.Sum(nil))
			if done.SHA256 != sum {
				return nil, fmt.Errorf("checksum mismatch")
			}
			return buf, nil
		default:
			return nil, fmt.Errorf("unexpected message %d", typ)
		}
	}
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := path[:len(path)-len(ext)]
	for i := 1; i < 1000; i++ {
		cand := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
	return fmt.Sprintf("%s-%d%s", base, time.Now().Unix(), ext)
}
