package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"beam/internal/client"
	"beam/internal/clipboard"
	"beam/internal/device"
	"beam/internal/portal"
	"beam/internal/transfer"
)

func cmdPortal() *cobra.Command {
	var noBrowser bool
	var tray string
	c := &cobra.Command{
		Use:   "portal",
		Short: "Open the hand-controlled portal for sending to paired devices",
		RunE: func(cmd *cobra.Command, args []string) error {
			ident, err := ensureReady(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			fmt.Fprintln(cmd.OutOrStdout(), "BEAM portal")
			return portal.Run(ctx, portal.Options{
				Self: ident.Config.Name,
				List: func(ctx context.Context) ([]portal.Device, error) { return portalDevices(ctx, ident) },
				SendFile: func(ctx context.Context, id, path, name string, report func(int64, int64)) error {
					return portalSendFile(ctx, ident, id, path, name, report)
				},
				SendText: func(ctx context.Context, id, text string, report func(int64, int64)) error {
					return portalSendText(ctx, ident, id, text, report)
				},
				SendClip: func(ctx context.Context, id string, report func(int64, int64)) error {
					item, err := currentClipboard(clipboard.New(clipboard.NewNative(), nil))
					if err != nil {
						return err
					}
					target, err := portalTarget(ident, id)
					if err != nil {
						return err
					}
					return client.SendClipboard(ctx, ident, target, item, func(p transfer.Progress) {
						if report != nil {
							report(p.Bytes, p.Total)
						}
					})
				},
				Open:      portal.OpenBrowser,
				NoBrowser: noBrowser,
				Tray:      tray,
			})
		},
	}
	c.Flags().BoolVar(&noBrowser, "no-browser", false, "print the local address without opening a browser")
	c.Flags().StringVar(&tray, "tray", "", "directory of files and folders the hand can grab")
	return c
}

func portalDevices(ctx context.Context, ident *device.Identity) ([]portal.Device, error) {
	online, _ := browseDevices(ctx)
	items, err := device.ListDevices(ident, online)
	if err != nil {
		return nil, err
	}
	out := []portal.Device{}
	for _, item := range items {
		if item.Self || item.Peer == nil {
			continue
		}
		status := "offline"
		switch {
		case strings.HasPrefix(item.Status, "online"):
			status = "online"
		case item.Status == "unreachable":
			status = "unreachable"
		}
		out = append(out, portal.Device{ID: item.ID, Name: item.Name, Status: status})
	}
	return out, nil
}

func portalTarget(ident *device.Identity, id string) (*device.Listed, error) {
	items, err := portalDeviceList(ident)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id && items[i].Peer != nil && !items[i].Self {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("select a paired device")
}

func portalDeviceList(ident *device.Identity) ([]device.Listed, error) {
	online, _ := browseDevices(context.Background())
	return device.ListDevices(ident, online)
}

func portalSendFile(ctx context.Context, ident *device.Identity, id, path, name string, report func(int64, int64)) error {
	target, err := portalTarget(ident, id)
	if err != nil {
		return err
	}
	return client.SendFileAs(ctx, ident, target, path, name, func(p transfer.Progress) {
		if report != nil {
			report(p.Bytes, p.Total)
		}
	})
}

func portalSendText(ctx context.Context, ident *device.Identity, id, text string, report func(int64, int64)) error {
	target, err := portalTarget(ident, id)
	if err != nil {
		return err
	}
	return client.SendClipboard(ctx, ident, target, clipboard.TextItem(text), func(p transfer.Progress) {
		if report != nil {
			report(p.Bytes, p.Total)
		}
	})
}
