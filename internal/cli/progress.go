package cli

import (
	"fmt"
	"time"
)

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
