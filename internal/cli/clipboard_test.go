package cli

import (
	"context"
	"strings"
	"testing"

	"beam/internal/clipboard"
	"beam/internal/device"
	"beam/internal/discovery"
	"beam/internal/history"
	"beam/internal/transfer"
)

func TestClipboardToSendsCurrentOSClipboard(t *testing.T) {
	setupCLIDaemonTest(t)
	const secret = "quarterly clipboard secret"
	restore := installClipboardHooks(t)
	defer restore()
	currentClipboard = func(*clipboard.Service) (*clipboard.Item, error) {
		return clipboard.TextItem(secret), nil
	}
	var sent *clipboard.Item
	sendClipboardItem = func(_ context.Context, _ *device.Identity, target *device.Listed, item *clipboard.Item, _ transfer.Reporter) error {
		if target.Name != "Windows-PC" {
			t.Fatalf("target = %s", target.Name)
		}
		sent = item
		return nil
	}
	browseDevices = func(context.Context) ([]discovery.Remote, error) {
		return []discovery.Remote{{ID: "win", Name: "Windows-PC", Type: "windows"}}, nil
	}
	store, err := history.Open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(history.Item{Kind: history.KindText, Text: "old history item"}); err != nil {
		t.Fatal(err)
	}
	store.Close()

	output, err := execute(t, "clipboard", "--to", "Windows-PC")
	if err != nil {
		t.Fatal(err)
	}
	if sent == nil || sent.Text != secret {
		t.Fatalf("sent %#v", sent)
	}
	for _, want := range []string{"Sending clipboard -> Windows-PC", "ok Clipboard sent"} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q\n%s", want, output)
		}
	}
	if strings.Contains(output, secret) || strings.Contains(output, "old history item") {
		t.Fatalf("clipboard contents or history were displayed:\n%s", output)
	}
}

func TestClipboardCopyCommandsStayDistinct(t *testing.T) {
	setupCLIDaemonTest(t)
	restore := installClipboardHooks(t)
	defer restore()
	currentClipboard = func(*clipboard.Service) (*clipboard.Item, error) {
		return clipboard.TextItem("current os clipboard"), nil
	}
	var sent string
	sendClipboardItem = func(_ context.Context, _ *device.Identity, _ *device.Listed, item *clipboard.Item, _ transfer.Reporter) error {
		sent = item.Text
		return nil
	}
	browseDevices = func(context.Context) ([]discovery.Remote, error) {
		return []discovery.Remote{{ID: "win", Name: "Windows-PC", Type: "windows"}}, nil
	}
	store, err := history.Open()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(history.Item{Kind: history.KindText, Text: "saved history"}); err != nil {
		t.Fatal(err)
	}
	store.Close()

	listed, err := execute(t, "clipboard", "--list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed, "saved history") || strings.Contains(listed, "Clipboard sent") {
		t.Fatalf("list output:\n%s", listed)
	}
	if _, err := execute(t, "clipboard", "--copy", "Hello", "--to", "Windows-PC"); err != nil {
		t.Fatal(err)
	}
	if sent != "Hello" {
		t.Fatalf("explicit copy sent %q", sent)
	}
}

func installClipboardHooks(t *testing.T) func() {
	t.Helper()
	oldRead := currentClipboard
	oldSend := sendClipboardItem
	return func() {
		currentClipboard = oldRead
		sendClipboardItem = oldSend
	}
}
