package daemon

import (
	"context"
	"log"
	"os"
	"time"

	"beam/internal/client"
	"beam/internal/clipboard"
	"beam/internal/device"
	"beam/internal/discovery"
	"beam/internal/history"
	"beam/internal/protocol"
	"beam/internal/queue"
	"beam/internal/server"
	"beam/internal/storage"
)

func Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopPath, err := storage.DaemonStopPath()
	if err != nil {
		return err
	}
	_ = os.Remove(stopPath)
	go watchStopRequest(ctx, cancel, stopPath)

	ident, err := device.Load()
	if err != nil {
		return err
	}
	downloads, err := storage.DownloadsDir()
	if err != nil {
		return err
	}
	store, err := history.Open()
	if err != nil {
		return err
	}
	defer store.Close()
	clip := clipboard.New(clipboard.NewNative(), store)
	go advertiseLoop(ctx, ident)
	go watchClipboard(ctx, clip)
	go processQueue(ctx, ident)

	return server.Serve(ctx, server.Options{
		Ident: ident, Downloads: downloads, History: store, Clip: clip,
		Prompt: func(string, protocol.Offer) bool { return true },
		Log:    func(s string) { log.Print(s) },
		Ready:  func() { go writeState(ctx) },
	})
}

func watchStopRequest(ctx context.Context, cancel context.CancelFunc, path string) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := os.Stat(path); err == nil {
				_ = os.Remove(path)
				cancel()
				return
			}
		}
	}
}

func advertiseLoop(ctx context.Context, ident *device.Identity) {
	for {
		adv, err := discovery.Advertise(discovery.Info{
			ID: ident.Config.DeviceID, Name: ident.Config.Name,
			Type: string(ident.Config.Type), Port: ident.Config.ListenPort,
		}, false)
		if err == nil {
			<-ctx.Done()
			adv.Close()
			return
		}
		log.Printf("mDNS advertisement unavailable; retrying: %v", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
}

func watchClipboard(ctx context.Context, clip *clipboard.Service) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = clip.Snapshot()
		}
	}
}

func processQueue(ctx context.Context, ident *device.Identity) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		deliverQueued(ctx, ident)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func deliverQueued(ctx context.Context, ident *device.Identity) {
	entries, err := queue.List()
	if err != nil {
		return
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		peer, err := device.PeerByID(entry.TargetID)
		if err != nil {
			continue
		}
		remote, err := discovery.FindID(ctx, entry.TargetID)
		if err != nil {
			continue
		}
		target := &device.Listed{
			ID: entry.TargetID, Name: entry.TargetName, Type: peer.Type,
			Status: "online", Addr: remote.Addr, Peer: peer,
		}
		if err := client.SendFileAs(ctx, ident, target, entry.Payload, entry.Name, nil); err == nil {
			if err := queue.Remove(entry); err == nil {
				log.Printf("delivered queued file %q to %s", entry.Name, entry.TargetName)
			}
		}
	}
}
