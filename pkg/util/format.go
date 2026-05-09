package util

import (
	"fmt"
	"time"
)

// FormatBytes returns a human-readable byte size string (e.g. "4.7 GB").
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// FormatDuration returns a compact duration string like "1h02m30s" or "45s".
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// FormatSpeed returns a human-readable speed string in MB/s.
func FormatSpeed(bytesPerSec float64) string {
	mbps := bytesPerSec / 1024.0 / 1024.0
	if mbps >= 1 {
		return fmt.Sprintf("%.2f MB/s", mbps)
	}
	kbps := bytesPerSec / 1024.0
	return fmt.Sprintf("%.1f KB/s", kbps)
}
