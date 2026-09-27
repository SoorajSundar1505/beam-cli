package pairing

import (
	"context"
	"fmt"
	"net"
	"time"

	"beam/internal/crypto"
	"beam/internal/device"
	"beam/internal/discovery"
	"beam/internal/protocol"
	"beam/internal/transport"
)

func Host(ident *device.Identity) (code string, wait func(ctx context.Context) (*device.Peer, error), stop func(), err error) {
	code, err = crypto.RandomDigits(6)
	if err != nil {
		return "", nil, nil, err
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", ident.Config.ListenPort))
	if err != nil {
		return "", nil, nil, fmt.Errorf("listen: %w", err)
	}
	adv, err := discovery.Advertise(discovery.Info{
		ID:   ident.Config.DeviceID,
		Name: ident.Config.Name,
		Type: string(ident.Config.Type),
		Port: ident.Config.ListenPort,
	}, true)
	if err != nil {
		_ = ln.Close()
		return "", nil, nil, fmt.Errorf("advertise: %w", err)
	}
	stop = func() {
		adv.Close()
		_ = ln.Close()
	}
	wait = func(ctx context.Context) (*device.Peer, error) {
		type result struct {
			p   *device.Peer
			err error
		}
		ch := make(chan result, 1)
		go func() {
			conn, err := ln.Accept()
			if err != nil {
				ch <- result{err: err}
				return
			}
			defer conn.Close()
			bc, err := transport.HandshakeResponder(conn, ident, protocol.ModePair, code, nil)
			if err != nil {
				ch <- result{err: err}
				return
			}
			defer bc.Close()
			p := peerFromHello(bc.Remote)
			if err := device.UpsertPeer(p); err != nil {
				ch <- result{err: err}
				return
			}
			ch <- result{p: &p}
		}()
		select {
		case <-ctx.Done():
			_ = ln.Close()
			return nil, ctx.Err()
		case r := <-ch:
			return r.p, r.err
		}
	}
	return code, wait, stop, nil
}

func Join(ctx context.Context, ident *device.Identity, code string, target *discovery.Remote) (*device.Peer, error) {
	if target == nil {
		found, err := discovery.FindPairing(ctx)
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("no device is waiting to pair (run beam pair on the other device first)")
		}
		if len(found) > 1 {
			return nil, fmt.Errorf("multiple devices are pairing; specify one")
		}
		target = &found[0]
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	raw, err := d.DialContext(ctx, "tcp", target.Addr)
	if err != nil {
		return nil, err
	}
	defer raw.Close()
	bc, err := transport.HandshakeInitiator(raw, ident, protocol.ModePair, code, nil)
	if err != nil {
		return nil, err
	}
	defer bc.Close()
	p := peerFromHello(bc.Remote)
	if err := device.UpsertPeer(p); err != nil {
		return nil, err
	}
	return &p, nil
}

func peerFromHello(h protocol.Hello) device.Peer {
	return device.Peer{
		ID:        h.DeviceID,
		Name:      h.Name,
		Type:      device.Type(h.Type),
		PublicKey: crypto.EncodePublicKey(h.PublicKey),
		PairedAt:  time.Now().UTC(),
	}
}
