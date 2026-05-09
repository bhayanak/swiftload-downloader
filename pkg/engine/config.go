package engine

import (
	"net/http"
	"time"
)

const (
	DefaultWorkers   = 16
	DefaultBufSizeMB = 4
	DefaultRetries   = 3
	MinChunkSize     = 1 * 1024 * 1024 // 1 MB
)

// DownloadConfig holds all parameters for a download.
type DownloadConfig struct {
	URL          string
	OutputPath   string
	Workers      int           // 0 = auto (defaults to DefaultWorkers)
	Retries      int
	BufSize      int64         // bytes per read buffer; 0 = default
	SpeedLimit   int64         // bytes/sec, 0 = unlimited
	UseProxy     bool
	Checksum     string        // expected hash (optional)
	ChecksumAlgo string        // "md5", "sha256"
	Headers      http.Header   // custom request headers
	Parallel     bool          // enable parallel chunked download
}

// DownloadStatus represents the current state of a download.
type DownloadStatus int

const (
	StatusQueued DownloadStatus = iota
	StatusDownloading
	StatusPaused
	StatusCompleted
	StatusFailed
	StatusCancelled
)

func (s DownloadStatus) String() string {
	switch s {
	case StatusQueued:
		return "Queued"
	case StatusDownloading:
		return "Downloading"
	case StatusPaused:
		return "Paused"
	case StatusCompleted:
		return "Completed"
	case StatusFailed:
		return "Failed"
	case StatusCancelled:
		return "Cancelled"
	default:
		return "Unknown"
	}
}

// ProgressInfo is passed to progress callbacks.
type ProgressInfo struct {
	Downloaded   int64
	TotalSize    int64
	Speed        float64       // bytes/sec
	ETA          time.Duration
	Status       DownloadStatus
	ActiveChunks int
	Percent      float64
}

// ProgressFunc is called periodically with download progress.
type ProgressFunc func(ProgressInfo)

// Defaults fills in zero-value fields with defaults.
func (c *DownloadConfig) Defaults() {
	if c.Workers <= 0 {
		c.Workers = DefaultWorkers
	}
	if c.Retries < 0 {
		c.Retries = DefaultRetries
	}
	if c.BufSize <= 0 {
		c.BufSize = int64(DefaultBufSizeMB) * 1024 * 1024
	}
}
