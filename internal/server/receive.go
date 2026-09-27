package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"beam/internal/clipboard"
	"beam/internal/device"
	"beam/internal/history"
	"beam/internal/protocol"
	"beam/internal/transfer"
	"beam/internal/transport"
)

type FilePrompt func(from string, offer protocol.Offer) bool

type Options struct {
	Ident     *device.Identity
	Downloads string
	History   *history.Store
	Clip      *clipboard.Service
	Prompt    FilePrompt
	Progress  transfer.Reporter
	Log       func(string)
	Out       io.Writer
	In        io.Reader
	Ready     func()
}

func Serve(ctx context.Context, opt Options) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", opt.Ident.Config.ListenPort))
	if err != nil {
		return err
	}
	if opt.Ready != nil {
		opt.Ready()
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	var active sync.WaitGroup
	for {
		raw, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				active.Wait()
				return nil
			}
			return err
		}
		active.Add(1)
		go func(raw net.Conn) {
			defer active.Done()
			defer raw.Close()
			done := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					_ = raw.Close()
				case <-done:
				}
			}()
			if err := HandleConnContext(ctx, raw, opt); err != nil && ctx.Err() == nil && opt.Log != nil {
				opt.Log(err.Error())
			}
			close(done)
		}(raw)
	}
}

func HandleConn(raw net.Conn, opt Options) error {
	return HandleConnContext(context.Background(), raw, opt)
}

func HandleConnContext(ctx context.Context, raw net.Conn, opt Options) error {
	c, err := transport.HandshakeResponder(raw, opt.Ident, protocol.ModeData, "", func(id string) (*device.Peer, error) {
		return device.PeerByID(id)
	})
	if err != nil {
		return err
	}
	defer c.Close()
	if c.Remote.Mode != protocol.ModeData {
		return fmt.Errorf("rejected non-data session")
	}
	return handleData(ctx, c, opt)
}

func handleData(ctx context.Context, c *transport.Conn, opt Options) error {
	typ, payload, err := c.Receive()
	if err != nil {
		return err
	}
	if typ != protocol.MsgOffer {
		return fmt.Errorf("expected offer")
	}
	var offer protocol.Offer
	if err := json.Unmarshal(payload, &offer); err != nil {
		return err
	}
	from := c.Remote.Name
	if offer.Kind == protocol.KindClipboard {
		return receiveClipboard(ctx, c, opt, from, offer)
	}
	return receiveFile(ctx, c, opt, from, offer)
}

func receiveFile(ctx context.Context, c *transport.Conn, opt Options, from string, offer protocol.Offer) error {
	name, err := protocol.SafeFileName(offer.Name)
	if err != nil {
		_ = c.SendJSON(protocol.MsgReject, protocol.IDMsg{ID: offer.ID, Reason: "invalid filename"})
		return err
	}
	offer.Name = name
	if opt.Out != nil {
		fmt.Fprintf(opt.Out, "\nFrom: %s\nFile: %s\nSize: %s\n\n", from, name, FormatSize(offer.Size))
	}
	ok := true
	if opt.Prompt != nil {
		ok = opt.Prompt(from, offer)
	} else if opt.In != nil && opt.Out != nil {
		ok = defaultPrompt(opt.In, opt.Out)
	}
	if !ok {
		_ = c.SendJSON(protocol.MsgReject, protocol.IDMsg{ID: offer.ID, Reason: "declined"})
		if opt.Out != nil {
			fmt.Fprintln(opt.Out, "Declined.")
		}
		return nil
	}
	path, err := transfer.ReceiveToFile(ctx, c, offer, opt.Downloads, opt.Progress)
	if err != nil {
		if opt.Out != nil {
			fmt.Fprintf(opt.Out, "✗ Transfer failed: %v\n", err)
		}
		return err
	}
	if opt.Out != nil {
		fmt.Fprintf(opt.Out, "✓ Saved to %s\n", path)
	}
	return nil
}

func receiveClipboard(ctx context.Context, c *transport.Conn, opt Options, from string, offer protocol.Offer) error {
	data, err := transfer.ReceiveToBuffer(ctx, c, offer, clipboard.MaxClipBytes, opt.Progress)
	if err != nil {
		return err
	}
	it := &clipboard.Item{
		MIME:     offer.MIME,
		Filename: offer.Name,
	}
	switch offer.ClipFormat {
	case protocol.ClipImage:
		it.Kind = history.KindImage
		it.Data = data
	case protocol.ClipFile:
		it.Kind = history.KindFile
		it.Data = data
	default:
		it.Kind = history.KindText
		it.Text = string(data)
		it.Data = nil
	}
	if opt.Clip != nil {
		if err := opt.Clip.Write(it); err != nil {
			return err
		}
		_, _ = opt.Clip.RecordIncoming(it)
	} else if it.Kind != history.KindText {
		dir := opt.Downloads
		name := offer.Name
		if name == "" {
			name = "clipboard.bin"
		}
		safe, err := protocol.SafeFileName(name)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, safe)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return err
		}
	}
	if opt.Out != nil {
		fmt.Fprintf(opt.Out, "✓ Copied to %s clipboard (from %s).\n", opt.Ident.Config.Name, from)
	}
	return nil
}

func defaultPrompt(in io.Reader, out io.Writer) bool {
	fmt.Fprint(out, "Accept? [Y/n] ")
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		return true
	}
	s := strings.TrimSpace(strings.ToLower(sc.Text()))
	return s == "" || s == "y" || s == "yes"
}

func FormatSize(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.0f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.0f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func DialPeer(ctx context.Context, ident *device.Identity, peer *device.Peer, addr string) (*transport.Conn, error) {
	d := net.Dialer{Timeout: 10 * time.Second}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	c, err := transport.HandshakeInitiator(raw, ident, protocol.ModeData, "", peer)
	if err != nil {
		raw.Close()
		return nil, err
	}
	return c, nil
}
