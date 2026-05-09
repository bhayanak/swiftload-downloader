package engine

import (
	"testing"
	"time"
)

func TestDownloadConfigDefaults(t *testing.T) {
	c := DownloadConfig{}
	c.Defaults()

	if c.Workers != DefaultWorkers {
		t.Errorf("Workers: got %d, want %d", c.Workers, DefaultWorkers)
	}
	if c.Retries != 0 {
		// Retries defaults only when < 0; zero means "unlimited" is intentional.
		t.Errorf("Retries should stay 0 when not negative, got %d", c.Retries)
	}
	if c.BufSize != int64(DefaultBufSizeMB)*1024*1024 {
		t.Errorf("BufSize: got %d, want %d", c.BufSize, int64(DefaultBufSizeMB)*1024*1024)
	}
}

func TestDownloadConfigDefaults_NegativeRetries(t *testing.T) {
	c := DownloadConfig{Retries: -5}
	c.Defaults()
	if c.Retries != DefaultRetries {
		t.Errorf("expected Retries to be reset to %d, got %d", DefaultRetries, c.Retries)
	}
}

func TestDownloadConfigDefaults_PreserveExisting(t *testing.T) {
	c := DownloadConfig{Workers: 8, BufSize: 2 * 1024 * 1024, Retries: 5}
	c.Defaults()
	if c.Workers != 8 {
		t.Errorf("expected Workers=8, got %d", c.Workers)
	}
	if c.BufSize != 2*1024*1024 {
		t.Errorf("expected BufSize=2MB, got %d", c.BufSize)
	}
	if c.Retries != 5 {
		t.Errorf("expected Retries=5, got %d", c.Retries)
	}
}

func TestDownloadStatusString(t *testing.T) {
	tests := []struct {
		status DownloadStatus
		want   string
	}{
		{StatusQueued, "Queued"},
		{StatusDownloading, "Downloading"},
		{StatusPaused, "Paused"},
		{StatusCompleted, "Completed"},
		{StatusFailed, "Failed"},
		{StatusCancelled, "Cancelled"},
		{DownloadStatus(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("DownloadStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestProgressTrackerAddBytes(t *testing.T) {
	p := newProgressTracker(1000)
	p.addBytes(100)
	p.addBytes(200)
	if got := p.getDownloaded(); got != 300 {
		t.Errorf("expected 300 bytes downloaded, got %d", got)
	}
}

func TestProgressTrackerStatus(t *testing.T) {
	p := newProgressTracker(0)
	if p.getStatus() != StatusQueued {
		t.Errorf("initial status should be StatusQueued")
	}
	p.setStatus(StatusDownloading)
	if p.getStatus() != StatusDownloading {
		t.Error("expected StatusDownloading after setStatus")
	}
}

func TestProgressTrackerCallback(t *testing.T) {
	p := newProgressTracker(1000)
	p.startTime = p.startTime.Add(-time.Second) // simulate 1s elapsed

	var received ProgressInfo
	called := make(chan struct{}, 1)
	p.addCallback(func(info ProgressInfo) {
		received = info
		select {
		case called <- struct{}{}:
		default:
		}
	})

	p.addBytes(500)
	p.setStatus(StatusDownloading)
	p.notify()

	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("callback was not called")
	}

	if received.Downloaded != 500 {
		t.Errorf("expected Downloaded=500, got %d", received.Downloaded)
	}
	if received.TotalSize != 1000 {
		t.Errorf("expected TotalSize=1000, got %d", received.TotalSize)
	}
	if received.Percent < 49 || received.Percent > 51 {
		t.Errorf("expected Percent≈50, got %.2f", received.Percent)
	}
	if received.Speed <= 0 {
		t.Errorf("expected positive speed, got %v", received.Speed)
	}
}

func TestProgressTrackerTicker(t *testing.T) {
	p := newProgressTracker(0)
	p.setStatus(StatusDownloading)

	fired := make(chan struct{}, 5)
	p.addCallback(func(_ ProgressInfo) {
		select {
		case fired <- struct{}{}:
		default:
		}
	})

	stop := p.startTicker(50 * time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	stop()

	if len(fired) == 0 {
		t.Error("expected ticker to have fired at least once")
	}
}

func TestProgressTrackerSetDownloaded(t *testing.T) {
	p := newProgressTracker(1000)
	p.setDownloaded(750)
	if got := p.getDownloaded(); got != 750 {
		t.Errorf("expected 750, got %d", got)
	}
}
