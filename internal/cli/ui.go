package cli

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"unicode/utf8"

	"beam/internal/device"
	"beam/internal/server"
	"beam/internal/transfer"
)

const (
	markOnline  = "*"
	markOffline = "o"
)

func headline(out io.Writer, running bool) string {
	if running {
		return "BEAM " + markOnline + " " + paint(out, "ONLINE", "32")
	}
	return "BEAM " + markOffline + " " + paint(out, "OFFLINE", "31")
}

func deviceState(status string) string {
	if strings.HasPrefix(status, "online") || status == "this device" {
		label := "online"
		if status == "this device" {
			label = "this device"
		}
		return markOnline + " " + label
	}
	return markOffline + " offline"
}

func renderDevices(out io.Writer, running bool, name string, details []string, items []device.Listed) {
	fmt.Fprintln(out, headline(out, running))
	fmt.Fprintf(out, "Device: %s\n", name)
	for _, detail := range details {
		fmt.Fprintln(out, detail)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Nearby devices")
	text := formatListed(items, true)
	if text == "" {
		fmt.Fprintln(out, "(none)")
		return
	}
	fmt.Fprint(out, text)
}

func renderChoices(out io.Writer, items []device.Listed) {
	fmt.Fprintln(out, "Select device")
	fmt.Fprintln(out)
	fmt.Fprint(out, formatListed(items, false))
	fmt.Fprintln(out)
	fmt.Fprint(out, "Enter number: ")
}

func formatListed(items []device.Listed, nearbyOnly bool) string {
	var rows []row
	number := 0
	for _, item := range items {
		if nearbyOnly && item.Self {
			continue
		}
		number++
		rows = append(rows, row{number: number, name: item.Name, state: deviceState(item.Status)})
	}
	return alignRows(rows)
}

func formatClipboard(entries []row) string {
	return alignRows(entries)
}

type row struct {
	number int
	name   string
	state  string
}

func alignRows(rows []row) string {
	if len(rows) == 0 {
		return ""
	}
	width := 0
	for _, item := range rows {
		if n := displayWidth(item.name); n > width {
			width = n
		}
	}
	var b strings.Builder
	for _, item := range rows {
		fmt.Fprintf(&b, "%d. %s  %s\n", item.number, pad(item.name, width), item.state)
	}
	return b.String()
}

func pad(value string, width int) string {
	gap := width - displayWidth(value)
	if gap < 0 {
		gap = 0
	}
	return value + strings.Repeat(" ", gap)
}

func displayWidth(value string) int {
	width := 0
	for _, r := range value {
		width += runeWidth(r)
	}
	return width
}

func runeWidth(r rune) int {
	if r < 0x20 || r == utf8.RuneError {
		return 0
	}
	// Wide East Asian characters occupy two terminal columns.
	if r >= 0x1100 && (r <= 0x115F || r == 0x2329 || r == 0x232A ||
		(r >= 0x2E80 && r <= 0xA4CF && r != 0x303F) ||
		(r >= 0xAC00 && r <= 0xD7A3) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE10 && r <= 0xFE19) ||
		(r >= 0xFE30 && r <= 0xFE6F) ||
		(r >= 0xFF00 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x1F300 && r <= 0x1FAFF)) {
		return 2
	}
	return 1
}

func ok(out io.Writer, message string) {
	fmt.Fprintln(out, "ok "+message)
}

func failed(out io.Writer, message string) {
	fmt.Fprintln(out, "x "+message)
}

type progressView struct {
	width int
}

func (v *progressView) Render(out io.Writer, title string, p transfer.Progress) {
	filled := 20 * p.Percent() / 100
	bar := strings.Repeat("#", filled) + strings.Repeat("-", 20-filled)
	eta := "-"
	if d := p.ETA(); d > 0 {
		eta = fmt.Sprintf("%ds", int(d.Seconds()+0.5))
	}
	line := fmt.Sprintf("%s  [%s] %3d%%  %s / %s  %s  ETA %s",
		title, bar, p.Percent(), server.FormatSize(p.Bytes), server.FormatSize(p.Total), formatSpeed(p.Speed()), eta)
	if extra := v.width - displayWidth(line); extra > 0 {
		line += strings.Repeat(" ", extra)
	}
	v.width = displayWidth(line)
	fmt.Fprintf(out, "\r%s", line)
	if p.Done || p.Err != nil {
		fmt.Fprintln(out)
		v.width = 0
	}
}

func formatSpeed(bps float64) string {
	return server.FormatSize(int64(bps)) + "/s"
}

func colorEnabled(out io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		return os.Getenv("WT_SESSION") != "" || strings.Contains(os.Getenv("TERM"), "xterm")
	}
	return os.Getenv("TERM") != ""
}

func paint(out io.Writer, text, code string) string {
	if !colorEnabled(out) {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}
