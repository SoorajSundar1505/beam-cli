package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
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

var (
	startDaemon            = daemon.Start
	stopDaemon             = daemon.Stop
	daemonStatus           = daemon.Status
	enableDaemonAutostart  = daemon.EnableAutostart
	disableDaemonAutostart = daemon.DisableAutostart
	daemonAutostartEnabled = daemon.AutostartEnabled
	browseDevices          = func(ctx context.Context) ([]discovery.Remote, error) {
		return discovery.Browse(ctx, 2*time.Second)
	}
	currentClipboard = func(s *clipboard.Service) (*clipboard.Item, error) {
		return s.ReadCurrent()
	}
	sendClipboardItem = client.SendClipboard
)

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "beam",
		Short: "Move files and clipboard between your devices on the local network",
		Long: `BEAM is a CLI-first, local-network tool for sending files and clipboard
content between your own devices. No cloud. No accounts.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return showDevices(cmd)
		},
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
			_, created, err := prepareDevice(name)
			if err != nil {
				return err
			}
			if !created {
				fmt.Fprintln(cmd.OutOrStdout(), "BEAM is already configured")
			}
			return activate(cmd.OutOrStdout(), true)
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
			ident, err := ensureReady(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if strings.TrimSpace(code) == "" {
				if err := stopDaemon(); err != nil {
					return fmt.Errorf("pause background receiver for pairing: %w", err)
				}
				restartOnExit := true
				defer func() {
					if restartOnExit {
						_ = ensureDaemon()
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
				if err := ensureDaemon(); err != nil {
					return fmt.Errorf("paired, but background receiver failed to restart: %w", err)
				}
				ok(cmd.OutOrStdout(), "Paired with "+p.Name)
				return nil
			}
			p, err := pairing.Join(ctx, ident, strings.TrimSpace(code), nil)
			if err != nil {
				return err
			}
			if err := ensureDaemon(); err != nil {
				return fmt.Errorf("paired, but background receiver failed to start: %w", err)
			}
			ok(cmd.OutOrStdout(), "Paired with "+p.Name)
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
			return showDevices(cmd)
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
			ident, err := ensureReady(cmd.OutOrStdout())
			if err != nil {
				return err
			}
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
			title := fmt.Sprintf("Sending %s -> %s", path, target.Name)
			fmt.Fprintln(cmd.OutOrStdout(), title)
			fmt.Fprintln(cmd.OutOrStdout())
			var view progressView
			last := time.Now()
			err = client.SendFile(ctx, ident, target, path, func(p transfer.Progress) {
				if time.Since(last) < 200*time.Millisecond && !p.Done && p.Err == nil {
					return
				}
				last = time.Now()
				view.Render(cmd.OutOrStdout(), title, p)
			})
			if err != nil {
				if errors.Is(err, client.ErrOffline) {
					return offerQueue(cmd, path, target)
				}
				fmt.Fprintln(cmd.OutOrStdout())
				failed(cmd.OutOrStdout(), "Transfer failed: "+err.Error())
				return err
			}
			ok(cmd.OutOrStdout(), "Transfer complete")
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
	ok(cmd.OutOrStdout(), fmt.Sprintf("Queued %s for %s (%s)", entry.Name, target.Name, server.FormatSize(entry.Size)))
	if err := ensureDaemon(); err != nil {
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
			ident, _, err := device.Ensure("")
			if err != nil {
				return err
			}
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
			var view progressView
			return server.Serve(ctx, server.Options{
				Ident:     ident,
				Downloads: dl,
				History:   store,
				Clip:      clip,
				Out:       cmd.OutOrStdout(),
				In:        cmd.InOrStdin(),
				Progress: func(p transfer.Progress) {
					view.Render(cmd.OutOrStdout(), "Receiving "+p.Name, p)
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
			if _, _, err := device.Ensure(""); err != nil {
				return err
			}
			if autostart {
				if err := enableDaemonAutostart(); err != nil {
					return fmt.Errorf("enable automatic startup: %w", err)
				}
			}
			if err := startDaemon(); err != nil {
				if errors.Is(err, daemon.ErrAlreadyRunning) {
					fmt.Fprintln(cmd.OutOrStdout(), "BEAM daemon is already running.")
					return nil
				}
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "BEAM daemon started.")
			if autostart {
				fmt.Fprintln(cmd.OutOrStdout(), "Automatic startup enabled")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&autostart, "autostart", false, "start BEAM automatically when you sign in (macOS/Windows)")
	return c
}

func prepareDevice(name string) (*device.Identity, bool, error) {
	return device.Ensure(name)
}

func activate(out io.Writer, announce bool) error {
	autostartReady := true
	if err := enableDaemonAutostart(); err != nil {
		if errors.Is(err, daemon.ErrAutostartUnsupported) {
			autostartReady = false
		} else {
			return fmt.Errorf("autostart setup failed: %w", err)
		}
	}
	if err := ensureDaemon(); err != nil {
		return fmt.Errorf("background receiver failed to start: %w", err)
	}
	if announce {
		ok(out, "BEAM is ready")
		ok(out, "Background receiver started")
		if autostartReady {
			ok(out, "Autostart enabled")
		} else {
			ok(out, "Autostart is unavailable on this platform")
		}
	}
	return nil
}

func ensureReady(out io.Writer) (*device.Identity, error) {
	ident, created, err := prepareDevice("")
	if err != nil {
		return nil, err
	}
	if err := activate(out, created); err != nil {
		return nil, err
	}
	return ident, nil
}

func showDevices(cmd *cobra.Command) error {
	ident, err := ensureReady(cmd.OutOrStdout())
	if err != nil {
		return err
	}
	_, running, err := daemonStatus()
	if err != nil {
		return err
	}
	online, err := browseDevices(cmd.Context())
	if err != nil {
		online = nil
	}
	items, err := device.ListDevices(ident, online)
	if err != nil {
		return err
	}
	renderDevices(cmd.OutOrStdout(), running, ident.Config.Name, nil, items)
	return nil
}

func ensureDaemon() error {
	err := startDaemon()
	if errors.Is(err, daemon.ErrAlreadyRunning) {
		return nil
	}
	return err
}

func cmdStop() *cobra.Command {
	var disableAutostart bool
	c := &cobra.Command{
		Use:   "stop",
		Short: "Stop the background receiver",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := stopDaemon(); err != nil {
				return err
			}
			if disableAutostart {
				if err := disableDaemonAutostart(); err != nil {
					return fmt.Errorf("disable automatic startup: %w", err)
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), "BEAM daemon stopped.")
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
			ident, err := device.Load()
			if err != nil {
				return err
			}
			online, err := browseDevices(cmd.Context())
			if err != nil {
				online = nil
			}
			items, err := device.ListDevices(ident, online)
			if err != nil {
				return err
			}
			state, running, err := daemonStatus()
			if err != nil {
				return err
			}
			autostart, err := daemonAutostartEnabled()
			if err != nil {
				return fmt.Errorf("check autostart status: %w", err)
			}
			details := []string{}
			if running {
				details = append(details, fmt.Sprintf("Daemon: running (PID %d)", state.PID))
			} else {
				details = append(details, "Daemon: stopped")
			}
			if autostart {
				details = append(details, "Autostart: enabled")
			} else {
				details = append(details, "Autostart: disabled")
			}
			if entries, err := queue.List(); err == nil {
				details = append(details, fmt.Sprintf("Queued transfers: %d", len(entries)))
			}
			renderDevices(cmd.OutOrStdout(), running, ident.Config.Name, details, items)
			return nil
		},
	}
}

func cmdDaemon() *cobra.Command {
	var background bool
	c := &cobra.Command{
		Use:   "daemon",
		Short: "Run the daemon in the foreground for debugging",
		RunE: func(cmd *cobra.Command, args []string) error {
			var logFile *os.File
			if background {
				path, err := storage.DaemonLogPath()
				if err != nil {
					return err
				}
				logFile, err = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
				if err != nil {
					return err
				}
				defer logFile.Close()
				log.SetOutput(logFile)
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			err := daemon.Run(ctx)
			if background && err != nil {
				log.Printf("daemon exited with error: %v", err)
			}
			return err
		},
	}
	c.Flags().BoolVar(&background, "background", false, "redirect daemon logs to the local BEAM log")
	_ = c.Flags().MarkHidden("background")
	return c
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
			ident, err := ensureReady(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			store, err := history.Open()
			if err != nil {
				return err
			}
			defer store.Close()
			clip := clipboard.New(clipboard.NewNative(), store)
			liveSend := strings.TrimSpace(to) != "" && copyID == "" && search == "" && !list
			if !liveSend {
				_, _ = clip.Snapshot()
			}

			switch {
			case copyID != "":
				target, err := targetForClipboard(cmd, ident, to)
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
			case list:
				items, err := store.List(50)
				if err != nil {
					return err
				}
				printClip(cmd, items)
				return nil
			case strings.TrimSpace(to) != "":
				target, err := targetForClipboard(cmd, ident, to)
				if err != nil {
					return err
				}
				return sendCurrentClipboard(cmd, ident, clip, target)
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
	c.Flags().StringVar(&to, "to", "", "target device; without --copy, sends the current clipboard")
	_ = list
	return c
}

func printClip(cmd *cobra.Command, items []history.Item) {
	if len(items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "(no clipboard history)")
		return
	}
	rows := make([]row, len(items))
	for i, item := range items {
		rows[i] = row{number: i + 1, name: clipboard.Preview(item), state: RelTime(item.CreatedAt)}
	}
	fmt.Fprint(cmd.OutOrStdout(), formatClipboard(rows))
}

func copyTo(cmd *cobra.Command, ident *device.Identity, clip *clipboard.Service, store *history.Store, copyID string, target *device.Listed) error {
	it, err := resolveCopy(clip, store, copyID)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintf(cmd.OutOrStdout(), "Sending clipboard -> %s\n", target.Name)
	if err := sendClipboardItem(ctx, ident, target, it, nil); err != nil {
		return err
	}
	ok(cmd.OutOrStdout(), "Copied to "+target.Name+" clipboard")
	return nil
}

func sendCurrentClipboard(cmd *cobra.Command, ident *device.Identity, clip *clipboard.Service, target *device.Listed) error {
	item, err := currentClipboard(clip)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintf(cmd.OutOrStdout(), "Sending clipboard -> %s\n", target.Name)
	if err := sendClipboardItem(ctx, ident, target, item, nil); err != nil {
		return err
	}
	ok(cmd.OutOrStdout(), "Clipboard sent")
	return nil
}

func targetForClipboard(cmd *cobra.Command, ident *device.Identity, to string) (*device.Listed, error) {
	items, err := listed(ident)
	if err != nil {
		return nil, err
	}
	return resolveOrSelect(cmd, to, items)
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
	online, _ := browseDevices(context.Background())
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
