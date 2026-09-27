package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"beam/internal/server"
	"beam/internal/transfer"
)

func PrintProgress(w io.Writer, title string, p transfer.Progress) {
	pct := p.Percent()
	width := 20
	filled := width * pct / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	speed := formatSpeed(p.Speed())
	eta := "—"
	if d := p.ETA(); d > 0 {
		eta = fmt.Sprintf("%ds", int(d.Seconds()+0.5))
	}
	fmt.Fprintf(w, "\r%s\n[%s] %d%%\n%s / %s\nSpeed: %s\nETA: %s\n",
		title, bar, pct, server.FormatSize(p.Bytes), server.FormatSize(p.Total), speed, eta)
}

func formatSpeed(bps float64) string {
	return server.FormatSize(int64(bps)) + "/s"
}

func RelTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		n := int(d.Minutes())
		if n == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d min ago", n)
	case d < 24*time.Hour:
		n := int(d.Hours())
		if n == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", n)
	default:
		n := int(d.Hours() / 24)
		if n == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", n)
	}
}
