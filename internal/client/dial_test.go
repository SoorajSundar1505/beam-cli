package client

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"beam/internal/device"
	"beam/internal/transport"
)

func testTarget(status, addr string) *device.Listed {
	return &device.Listed{
		ID: "win", Name: "Suraj-Windows", Status: status, Addr: addr,
		Peer: &device.Peer{ID: "win", Name: "Suraj-Windows", PublicKey: "public-key-windows", PairedAt: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)},
	}
}

func TestFailedCachedEndpointRefreshesOnce(t *testing.T) {
	t.Setenv("BEAM_CONFIG_DIR", t.TempDir())
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	if err := device.UpsertPeer(*testTarget("online", oldAddr).Peer); err != nil {
		t.Fatal(err)
	}
	lookups := 0
	dials := []string{}
	stale := errors.New("dial tcp 192.168.1.36:47821: i/o timeout")
	conn, err := connectPeer(context.Background(), testTarget("online", oldAddr), func(context.Context, string) (string, error) {
		lookups++
		return freshAddr, nil
	}, func(_ context.Context, addr string) (*transport.Conn, error) {
		dials = append(dials, addr)
		if addr == oldAddr {
			return nil, stale
		}
		return &transport.Conn{}, nil
	})
	if err != nil || conn == nil {
		t.Fatalf("retry: %v", err)
	}
	if lookups != 1 || len(dials) != 2 || dials[0] != oldAddr || dials[1] != freshAddr {
		t.Fatalf("lookups=%d dials=%v", lookups, dials)
	}
	stored, err := device.LoadPeers()
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Endpoint != freshAddr || stored[0].PublicKey != "public-key-windows" || stored[0].LastReachable.IsZero() {
		t.Fatalf("stored after retry: %+v", stored)
	}
}

func TestDialRetryIsBounded(t *testing.T) {
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	t.Setenv("BEAM_CONFIG_DIR", t.TempDir())
	dials := 0
	_, err := connectPeer(context.Background(), testTarget("online", oldAddr), func(context.Context, string) (string, error) {
		return oldAddr, nil
	}, func(context.Context, string) (*transport.Conn, error) {
		dials++
		return nil, errors.New("dial tcp 192.168.1.36:47821: connect: host is down")
	})
	if dials != maxDialAttempts {
		t.Fatalf("dials=%d", dials)
	}
	var down *UnreachableError
	if !errors.As(err, &down) {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(down.Error(), "dial tcp") || strings.Contains(down.Error(), "192.168.1.36") || strings.Contains(down.Error(), "i/o timeout") {
		t.Fatalf("user error leaked the dial failure: %s", down.Error())
	}
	if down.Error() != "Suraj-Windows is currently unreachable." {
		t.Fatalf("message: %s", down.Error())
	}
	if down.Cause == nil || !strings.Contains(down.Cause.Error(), "host is down") {
		t.Fatalf("debug cause missing: %+v", down)
	}
}

func TestMissingPeerStaysOffline(t *testing.T) {
	t.Setenv("BEAM_DATA_DIR", t.TempDir())
	t.Setenv("BEAM_CONFIG_DIR", t.TempDir())
	dials := 0
	_, err := connectPeer(context.Background(), testTarget("offline", ""), func(context.Context, string) (string, error) {
		return "", errors.New("not found")
	}, func(context.Context, string) (*transport.Conn, error) {
		dials++
		return nil, errors.New("should not dial")
	})
	if dials != 0 || !errors.Is(err, ErrOffline) {
		t.Fatalf("dials=%d err=%v", dials, err)
	}
}

const (
	oldAddr   = "192.168.1.36:47821"
	freshAddr = "192.168.1.50:47821"
)
