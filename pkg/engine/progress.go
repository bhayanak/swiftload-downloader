package engine

import (
	"sync"
	"sync/atomic"
	"time"
)

// progressTracker manages download progress and notifies callbacks.
type progressTracker struct {
	downloaded int64 // atomic
	totalSize  int64
	startTime  time.Time
	mu         sync.RWMutex
	callbacks  []ProgressFunc
	status     DownloadStatus
}

func newProgressTracker(totalSize int64) *progressTracker {
	return &progressTracker{
		totalSize: totalSize,
		startTime: time.Now(),
		status:    StatusQueued,
	}
}

func (p *progressTracker) addCallback(fn ProgressFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.callbacks = append(p.callbacks, fn)
}

func (p *progressTracker) setStatus(s DownloadStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = s
}

func (p *progressTracker) getStatus() DownloadStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

func (p *progressTracker) addBytes(n int64) {
	atomic.AddInt64(&p.downloaded, n)
}

func (p *progressTracker) getDownloaded() int64 {
	return atomic.LoadInt64(&p.downloaded)
}

func (p *progressTracker) setDownloaded(n int64) {
	atomic.StoreInt64(&p.downloaded, n)
}

func (p *progressTracker) info() ProgressInfo {
	downloaded := p.getDownloaded()
	elapsed := time.Since(p.startTime).Seconds()

	var speed float64
	if elapsed > 0.1 {
		speed = float64(downloaded) / elapsed
	}

	var eta time.Duration
	if speed > 0 && p.totalSize > 0 {
		remaining := float64(p.totalSize - downloaded)
		eta = time.Duration(remaining/speed) * time.Second
	}

	var pct float64
	if p.totalSize > 0 {
		pct = float64(downloaded) / float64(p.totalSize) * 100
	}

	return ProgressInfo{
		Downloaded: downloaded,
		TotalSize:  p.totalSize,
		Speed:      speed,
		ETA:        eta,
		Status:     p.getStatus(),
		Percent:    pct,
	}
}

func (p *progressTracker) notify() {
	info := p.info()
	p.mu.RLock()
	cbs := make([]ProgressFunc, len(p.callbacks))
	copy(cbs, p.callbacks)
	p.mu.RUnlock()

	for _, fn := range cbs {
		fn(info)
	}
}

// startTicker runs a background goroutine that fires progress callbacks
// at the given interval. Returns a stop function.
func (p *progressTracker) startTicker(interval time.Duration) func() {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				p.notify()
			}
		}
	}()
	return func() { close(done) }
}
