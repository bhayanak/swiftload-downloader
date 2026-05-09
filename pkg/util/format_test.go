package util

import (
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{5046586572, "4.7 GB"}, // 4.7 * 1024^3
		{1024 * 1024 * 1024, "1.0 GB"},
	}
	for _, tt := range tests {
		got := FormatBytes(tt.input)
		if got != tt.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{0, "0s"},
		{5 * time.Second, "5s"},
		{90 * time.Second, "1m30s"},
		{1*time.Hour + 2*time.Minute + 3*time.Second, "1h02m03s"},
		{59*time.Second + 500*time.Millisecond, "1m00s"}, // rounds up
	}
	for _, tt := range tests {
		got := FormatDuration(tt.input)
		if got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatSpeed(t *testing.T) {
	tests := []struct {
		bytesPerSec float64
		want        string
	}{
		{0, "0.0 KB/s"},
		{512, "0.5 KB/s"},
		{1024, "1.0 KB/s"},
		{1024 * 1024, "1.00 MB/s"},
		{5.5 * 1024 * 1024, "5.50 MB/s"},
	}
	for _, tt := range tests {
		got := FormatSpeed(tt.bytesPerSec)
		if got != tt.want {
			t.Errorf("FormatSpeed(%v) = %q, want %q", tt.bytesPerSec, got, tt.want)
		}
	}
}
