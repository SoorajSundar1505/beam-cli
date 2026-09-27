package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"beam/internal/client"
	"beam/internal/clipboard"
	"beam/internal/daemon"
	"beam/internal/device"
	"beam/internal/discovery"
	"beam/internal/history"
	"beam/internal/pairing"
	"beam/internal/queue"
	"beam/internal/server"
	"beam/internal/storage"
	"beam/internal/transfer"
)

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "beam",
		Short: "Move files and clipboard between your devices on the local network",
		Long: `BEAM is a CLI-first, local-network tool for sending files and clipboard
content between your own devices. No cloud. No accounts.`,
	}
	root.AddCommand(
		cmdInit(), cmdPair(), cmdDevices(), cmdSend(), cmdReceive(), cmdClipboard(),
		cmdStart(), cmdStop(), cmdStatus(), cmdDaemon(),
	)
	return root
}

func cmdInit() *cobra.Command {
	var name string
	c := &cobra.Command{
		Use:   "init",
		Short: "Create this device's identity and local configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			ident, err := device.Init(name)
			if err != nil {
				return err
			}
			cfg, _ := storage.ConfigDir()
			fmt.Fprintf(cmd.OutOrStdout(), "Initialized %s (%s)\n", ident.Config.Name, ident.Config.Type)
			fmt.Fprintf(cmd.OutOrStdout(), "Device ID: %s\n", ident.Config.DeviceID)
			fmt.Fprintf(cmd.OutOrStdout(), "Config:    %s\n", cfg)
			if err := daemon.Start(); err != nil {
				return fmt.Errorf("device initialized, but background receiver failed to start: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Background receiver started")
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "device display name (default: hostname)")
	return c
}

func cmdPair() *cobra.Command {
	var code string
	c := &cobra.Command{
		Use:   "pair",
		Short: "Pair another BEAM device on the local network",
		Long: `On the first device, run beam pair and share the 6-digit code.
On the second device, run beam pair --code NNNNNN.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ident := MustIdent()
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if strings.TrimSpace(code) == "" {
				if err := daemon.Stop(); err != nil {
					return fmt.Errorf("pause background receiver for pairing: %w", err)
				}
				restartOnExit := true
				defer func() {
					if restartOnExit {
						_ = daemon.Start()
					}
				}()
				gen, wait, cancel, err := pairing.Host(ident)
				if err != nil {
					return err
				}
				defer cancel()
				fmt.Fprintf(cmd.OutOrStdout(), "Pairing code: %s\n\nWaiting for the other device...\nOn the other device, run: beam pair --code %s\n", gen, gen)
				p, err := wait(ctx)
				if err != nil {
					return err
				}
				cancel()
				restartOnExit = false
				if err := daemon.Start(); err != nil {
					return fmt.Errorf("paired, but background receiver failed to restart: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "✓ Paired with %s\n", p.Name)
				return nil
			}
			p, err := pairing.Join(ctx, ident, strings.TrimSpace(code), nil)
			if err != nil {
				return err
			}
			if err := daemon.Start(); err != nil {
				return fmt.Errorf("paired, but background receiver failed to start: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Paired with %s\n", p.Name)
			return nil
		},
	}
	c.Flags().StringVar(&code, "code", "", "pairing code shown on the other device")
	return c
}

func cmdDevices() *cobra.Command {
	return &cobra.Command{
		Use:   "devices",
		Short: "List this device and paired peers",
		RunE: func(cmd *cobra.Command, args []string) error {
			ident := MustIdent()
			online, err := discovery.Browse(cmd.Context(), 2*time.Second)
			if err != nil {
				online = nil
			}
			items, err := device.ListDevices(ident, online)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), device.FormatTable(items))
			return nil
		},
	}
}

func cmdSend() *cobra.Command {
	var to string
	c := &cobra.Command{
		Use:   "send <file>",
		Short: "Send a file to a paired device",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ident := MustIdent()
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			items, err := listed(ident)
			if err != nil {
				return err
			}
			target, err := resolveOrSelect(cmd, to, items)
			if err != nil {
				return err
			}
			path := args[0]
			if target.Status == "offline" {
				return offerQueue(cmd, path, target)
			}
			title := fmt.Sprintf("Sending %s → %s", path, target.Name)
			fmt.Fprintln(cmd.OutOrStdout(), title)
			fmt.Fprintln(cmd.OutOrStdout())
			last := time.Now()
			err = client.SendFile(ctx, ident, target, path, func(p transfer.Progress) {
				if time.Since(last) < 200*time.Millisecond && !p.Done && p.Err == nil {
					return
				}
				last = time.Now()
				PrintProgress(cmd.OutOrStdout(), title, p)
			})
			if err != nil {
				if errors.Is(err, client.ErrOffline) {
					return offerQueue(cmd, path, target)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\n✗ Transfer failed: %v\n", err)
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\n✓ Transfer complete")
			return nil
		},
	}
	c.Flags().StringVar(&to, "to", "", "device name, id, or list number")
	return c
}

func offerQueue(cmd *cobra.Command, path string, target *device.Listed) error {
	fmt.Fprintf(cmd.OutOrStdout(), "%s is offline\n", target.Name)
	if !confirm(cmd.InOrStdin(), cmd.OutOrStdout(), "Queue transfer? [Y/n] ") {
		fmt.Fprintln(cmd.OutOrStdout(), "Transfer not queued.")
		return nil
	}
	if target.Peer == nil {
		return fmt.Errorf("device %s is not paired", target.Name)
	}
	entry, err := queue.EnqueueFile(path, target.ID, target.Name)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "✓ Queued %s for %s (%s)\n", entry.Name, target.Name, server.FormatSize(entry.Size))
	if err := daemon.Start(); err != nil {
		return fmt.Errorf("transfer queued, but background receiver is not running: %w", err)
	}
	return nil
}

func confirm(in io.Reader, out io.Writer, prompt string) bool {
	fmt.Fprint(out, prompt)
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		return true
	}
	answer := strings.ToLower(strings.TrimSpace(sc.Text()))
	return answer == "" || answer == "y" || answer == "yes"
}

func cmdReceive() *cobra.Command {
	return &cobra.Command{
		Use:   "receive",
		Short: "Wait for incoming files and clipboard transfers",
		RunE: func(cmd *cobra.Command, args []string) error {
			ident := MustIdent()
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			dl, err := storage.DownloadsDir()
			if err != nil {
				return err
			}
			store, err := history.Open()
			if err != nil {
				return err
			}
			defer store.Close()
			clip := clipboard.New(clipboard.NewNative(), store)
			go watchClipboard(ctx, clip)

			adv, err := discovery.Advertise(discovery.Info{
				ID:   ident.Config.DeviceID,
				Name: ident.Config.Name,
				Type: string(ident.Config.Type),
				Port: ident.Config.ListenPort,
			}, false)
			if err != nil {
				return fmt.Errorf("advertise: %w", err)
			}
			defer adv.Close()

			fmt.Fprintln(cmd.OutOrStdout(), "Waiting for incoming files...")
			return server.Serve(ctx, server.Options{
				Ident:     ident,
				Downloads: dl,
				History:   store,
				Clip:      clip,
				Out:       cmd.OutOrStdout(),
				In:        cmd.InOrStdin(),
				Progress: func(p transfer.Progress) {
					PrintProgress(cmd.OutOrStdout(), "Receiving "+p.Name, p)
				},
				Log: func(s string) {
					fmt.Fprintln(cmd.ErrOrStderr(), s)
				},
			})
		},
	}
}

func cmdStart() *cobra.Command {
	var autostart bool
	c := &cobra.Command{
		Use:   "start",
		Short: "Start the background receiver",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = MustIdent()
			if autostart {
				if err := daemon.EnableAutostart(); err != nil {
					return fmt.Errorf("enable automatic startup: %w", err)
				}
			}
			if err := daemon.Start(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "BEAM background receiver is running")
			if autostart {
				fmt.Fprintln(cmd.OutOrStdout(), "Automatic startup enabled")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&autostart, "autostart", false, "start BEAM automatically when you sign in (macOS/Windows)")
	return c
}

func cmdStop() *cobra.Command {
	var disableAutostart bool
	c := &cobra.Command{
		Use:   "stop",
		Short: "Stop the background receiver",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := daemon.Stop(); err != nil {
				return err
			}
			if disableAutostart {
				if err := daemon.DisableAutostart(); err != nil {
					return fmt.Errorf("disable automatic startup: %w", err)
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), "BEAM background receiver stopped")
			if disableAutostart {
				fmt.Fprintln(cmd.OutOrStdout(), "Automatic startup disabled")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&disableAutostart, "disable-autostart", false, "disable startup at sign-in")
	return c
}

func cmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show background receiver status",
		RunE: func(cmd *cobra.Command, args []string) error {
			state, running, err := daemon.Status()
			if err != nil {
				return err
			}
			if running {
				fmt.Fprintf(cmd.OutOrStdout(), "BEAM background receiver is running (PID %d)\n", state.PID)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "BEAM background receiver is stopped")
			}
			entries, err := queue.List()
			if err == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Queued transfers: %d\n", len(entries))
			}
			return nil
		},
	}
}

func cmdDaemon() *cobra.Command {
	return &cobra.Command{
		Use:    "daemon",
		Short:  "Run the background receiver in the foreground",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return daemon.Run(ctx)
		},
	}
}

func cmdClipboard() *cobra.Command {
	var (
		list   bool
		search string
		copyID string
		to     string
	)
	c := &cobra.Command{
		Use:   "clipboard",
		Short: "Local clipboard history and copy-to-device",
		RunE: func(cmd *cobra.Command, args []string) error {
			ident := MustIdent()
			store, err := history.Open()
			if err != nil {
				return err
			}
			defer store.Close()
			clip := clipboard.New(clipboard.NewNative(), store)
			_, _ = clip.Snapshot()

			switch {
			case copyID != "":
				if to == "" {
					items, err := listed(ident)
					if err != nil {
						return err
					}
					target, err := SelectDevice(cmd.InOrStdin(), cmd.OutOrStdout(), items)
					if err != nil {
						return err
					}
					return copyTo(cmd, ident, clip, store, copyID, target)
				}
				items, err := listed(ident)
				if err != nil {
					return err
				}
				target, err := device.ResolveTarget(to, items)
				if err != nil {
					return err
				}
				return copyTo(cmd, ident, clip, store, copyID, target)
			case search != "":
				items, err := store.Search(search, 50)
				if err != nil {
					return err
				}
				printClip(cmd, items)
				return nil
			default:
				items, err := store.List(50)
				if err != nil {
					return err
				}
				printClip(cmd, items)
				return nil
			}
		},
	}
	c.Flags().BoolVar(&list, "list", false, "show local clipboard history")
	c.Flags().StringVar(&search, "search", "", "search clipboard history")
	c.Flags().StringVar(&copyID, "copy", "", "history index or literal text to send")
	c.Flags().StringVar(&to, "to", "", "target device for --copy")
	_ = list
	return c
}

func printClip(cmd *cobra.Command, items []history.Item) {
	if len(items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "(no clipboard history)")
		return
	}
	for i, it := range items {
		fmt.Fprintf(cmd.OutOrStdout(), "%d  %-22s  %s\n", i+1, clipboard.Preview(it), RelTime(it.CreatedAt))
	}
}

func copyTo(cmd *cobra.Command, ident *device.Identity, clip *clipboard.Service, store *history.Store, copyID string, target *device.Listed) error {
	it, err := resolveCopy(clip, store, copyID)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintf(cmd.OutOrStdout(), "Sending clipboard item to %s...\n", target.Name)
	if err := client.SendClipboard(ctx, ident, target, it, nil); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "✓ Copied to %s clipboard.\n", target.Name)
	return nil
}

func resolveCopy(clip *clipboard.Service, store *history.Store, copyID string) (*clipboard.Item, error) {
	if n, err := strconv.Atoi(copyID); err == nil && n > 0 {
		items, err := store.List(50)
		if err != nil {
			return nil, err
		}
		if n <= len(items) {
			return clip.ItemFromHistory(&items[n-1])
		}
	}
	return clipboard.TextItem(copyID), nil
}

func listed(ident *device.Identity) ([]device.Listed, error) {
	online, _ := discovery.Browse(context.Background(), 2*time.Second)
	return device.ListDevices(ident, online)
}

func resolveOrSelect(cmd *cobra.Command, to string, items []device.Listed) (*device.Listed, error) {
	if strings.TrimSpace(to) == "" {
		return SelectDevice(cmd.InOrStdin(), cmd.OutOrStdout(), items)
	}
	return device.ResolveTarget(to, items)
}

func watchClipboard(ctx context.Context, clip *clipboard.Service) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = clip.Snapshot()
		}
	}
}
