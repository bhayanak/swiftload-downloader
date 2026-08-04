package engine

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bhayanak/swiftload-downloader/pkg/util"
)

const (
	progressInterval   = 500 * time.Millisecond
	resumeFlushInterval = 2 * time.Second
)

// Download manages a single file download with pause/resume/cancel support.
type Download struct {
	cfg      DownloadConfig
	client   *http.Client
	progress *progressTracker
	limiter  *rateLimiter
	cancel   context.CancelFunc
	mu       sync.Mutex
}

// NewDownload creates a new Download with the given config.
// Call Start() to begin downloading.
func NewDownload(cfg DownloadConfig) *Download {
	cfg.Defaults()
	transport := util.NewTransport(cfg.Workers, cfg.BufSize, cfg.UseProxy, cfg.ProxyURL)

	// Merge default browser-like headers with any user-supplied headers.
	headers := cfg.Headers
	if headers == nil {
		headers = make(http.Header)
	}
	if headers.Get("User-Agent") == "" {
		headers.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	}
	if headers.Get("Accept") == "" {
		headers.Set("Accept", "*/*")
	}
	if headers.Get("Accept-Language") == "" {
		headers.Set("Accept-Language", "en-US,en;q=0.9")
	}

	client := &http.Client{
		Timeout:   0,
		Transport: &headerTransport{base: transport, headers: headers, username: cfg.Username, password: cfg.Password},
	}

	return &Download{
		cfg:      cfg,
		client:   client,
		progress: newProgressTracker(0),
		limiter:  newRateLimiter(cfg.SpeedLimit),
	}
}

// headerTransport wraps an http.RoundTripper and injects default headers
// (and optional basic-auth credentials) into every outgoing request.
type headerTransport struct {
	base     http.RoundTripper
	headers  http.Header
	username string
	password string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, vs := range t.headers {
		if req.Header.Get(k) == "" {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
	}
	if t.username != "" && req.Header.Get("Authorization") == "" {
		req.SetBasicAuth(t.username, t.password)
	}
	return t.base.RoundTrip(req)
}

// OnProgress registers a callback that fires periodically with download progress.
func (d *Download) OnProgress(fn ProgressFunc) {
	d.progress.addCallback(fn)
}

// Status returns the current download status.
func (d *Download) Status() DownloadStatus {
	return d.progress.getStatus()
}

// Info returns the current progress info.
func (d *Download) Info() ProgressInfo {
	return d.progress.info()
}

// Cancel stops the download.
func (d *Download) Cancel() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel != nil {
		d.cancel()
	}
	d.progress.setStatus(StatusCancelled)
}

// Start begins the download. It blocks until the download completes or fails.
func (d *Download) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.cancel = cancel
	d.mu.Unlock()
	defer cancel()

	d.progress.setStatus(StatusDownloading)
	d.progress.startTime = time.Now()

	stopTicker := d.progress.startTicker(progressInterval)
	defer stopTicker()

	var err error
	if d.cfg.Parallel {
		err = d.parallelDownload(ctx)
	} else {
		err = serialDownload(ctx, d.client, d.cfg.URL, d.cfg.OutputPath, d.cfg.BufSize, d.progress, d.limiter)
	}

	if err != nil {
		if !errors.Is(err, context.Canceled) {
			d.progress.setStatus(StatusFailed)
		}
		d.progress.notify()
		return err
	}

	// Checksum verification.
	if d.cfg.Checksum != "" {
		if verifyErr := d.verifyChecksum(); verifyErr != nil {
			d.progress.setStatus(StatusFailed)
			d.progress.notify()
			return verifyErr
		}
	}

	d.progress.setStatus(StatusCompleted)
	d.progress.notify()
	removeResumeState(d.cfg.OutputPath)
	return nil
}

// parallelDownload runs a multi-chunk parallel download with resume support.
func (d *Download) parallelDownload(ctx context.Context) error {
	// HEAD to learn file size and whether the server accepts Range requests.
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, d.cfg.URL, nil)
	if err != nil {
		return fmt.Errorf("HEAD request failed: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("HEAD request failed: %w", err)
	}
	resp.Body.Close()

	// Use the final URL after redirects for chunk downloads.
	downloadURL := resp.Request.URL.String()

	size, parseErr := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if parseErr != nil || size <= 0 {
		return serialDownload(ctx, d.client, downloadURL, d.cfg.OutputPath, d.cfg.BufSize, d.progress, d.limiter)
	}

	if !strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes") {
		return serialDownload(ctx, d.client, downloadURL, d.cfg.OutputPath, d.cfg.BufSize, d.progress, d.limiter)
	}

	d.progress.totalSize = size
	etag := resp.Header.Get("ETag")
	lastModified := resp.Header.Get("Last-Modified")

	// Clamp worker count so each chunk is at least MinChunkSize bytes.
	workers := d.cfg.Workers
	if chunkFloor := int(size / MinChunkSize); chunkFloor < workers {
		workers = chunkFloor
		if workers < 1 {
			workers = 1
		}
	}

	// Try to resume from existing state.
	state, _ := loadResumeState(d.cfg.OutputPath)
	var resuming bool
	if state != nil && state.isResumable(etag, lastModified, size) {
		resuming = true
		d.progress.setDownloaded(state.totalDownloaded())
		// Use resolved URL from state if available.
		if state.URL != "" {
			downloadURL = state.URL
		}
	} else {
		state = newResumeState(downloadURL, d.cfg.OutputPath, size, etag, lastModified, workers)
	}

	// Open or create the output file.
	var file *os.File
	if resuming {
		file, err = os.OpenFile(d.cfg.OutputPath, os.O_RDWR, 0644)
	} else {
		file, err = os.Create(d.cfg.OutputPath)
	}
	if err != nil {
		return fmt.Errorf("cannot open output file: %w", err)
	}
	defer file.Close()

	if !resuming {
		if err := file.Truncate(size); err != nil {
			return fmt.Errorf("file pre-allocation failed: %w", err)
		}
	}

	// Save initial state.
	_ = saveResumeState(state)

	// Per-chunk atomic progress counters, seeded from any resumed bytes.
	// These are synced into state.Chunks[i].Downloaded under d.mu before each
	// save so partial (mid-chunk) progress survives a pause/crash.
	chunkProgress := make([]int64, len(state.Chunks))
	for i := range state.Chunks {
		chunkProgress[i] = state.Chunks[i].Downloaded
	}
	syncChunkProgress := func() {
		for i := range state.Chunks {
			if !state.Chunks[i].Done {
				state.Chunks[i].Downloaded = atomic.LoadInt64(&chunkProgress[i])
			}
		}
	}

	// Periodic resume state flusher.
	flushDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(resumeFlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-flushDone:
				return
			case <-ticker.C:
				d.mu.Lock()
				syncChunkProgress()
				_ = saveResumeState(state)
				d.mu.Unlock()
			}
		}
	}()

	// Determine which chunks to download.
	chunksToDownload := state.incompleteChunks()
	if len(chunksToDownload) == 0 {
		close(flushDone)
		return nil // all chunks already done
	}

	// Build the pool of source URLs: primary first, then any valid mirrors.
	// Chunks are spread across mirrors round-robin to parallelise across hosts.
	urls := []string{downloadURL}
	for _, m := range d.cfg.Mirrors {
		if s := strings.TrimSpace(m); s != "" {
			urls = append(urls, s)
		}
	}

	errCh := make(chan error, len(chunksToDownload))
	var wg sync.WaitGroup

	for i, idx := range chunksToDownload {
		wg.Add(1)
		chunkIdx := idx
		chunk := &state.Chunks[chunkIdx]
		startOffset := chunk.Start + chunk.Downloaded
		chunkURL := urls[i%len(urls)]

		go func() {
			defer wg.Done()
			err := downloadChunkWithRetry(
				ctx, d.client, chunkURL, file,
				startOffset, chunk.End,
				d.cfg.Retries, d.cfg.BufSize,
				&d.progress.downloaded,
				&chunkProgress[chunkIdx],
				d.limiter,
				func(_, _ int64, _, _ int, _ error, _ time.Duration) {
					d.progress.addRetry()
				},
			)
			if err != nil {
				errCh <- err
				return
			}
			d.mu.Lock()
			full := chunk.End - chunk.Start + 1
			atomic.StoreInt64(&chunkProgress[chunkIdx], full)
			chunk.Downloaded = full
			chunk.Done = true
			d.mu.Unlock()
		}()
	}

	wg.Wait()
	close(flushDone)
	close(errCh)

	// Collect errors.
	var errs []error
	for e := range errCh {
		errs = append(errs, e)
	}

	// Final state save (persist any mid-chunk progress on cancel/failure).
	d.mu.Lock()
	syncChunkProgress()
	_ = saveResumeState(state)
	d.mu.Unlock()

	if len(errs) > 0 {
		return fmt.Errorf("parallel download failed: %w", errors.Join(errs...))
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("file sync failed: %w", err)
	}
	return nil
}

// verifyChecksum checks the downloaded file against the expected checksum.
func (d *Download) verifyChecksum() error {
	var h hash.Hash
	switch strings.ToLower(d.cfg.ChecksumAlgo) {
	case "md5":
		h = md5.New()
	case "sha256", "":
		h = sha256.New()
	default:
		return fmt.Errorf("unsupported checksum algorithm: %s", d.cfg.ChecksumAlgo)
	}

	f, err := os.Open(d.cfg.OutputPath)
	if err != nil {
		return fmt.Errorf("checksum: cannot open file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("checksum: read error: %w", err)
	}

	got := fmt.Sprintf("%x", h.Sum(nil))
	if !strings.EqualFold(got, d.cfg.Checksum) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", d.cfg.Checksum, got)
	}
	return nil
}

// ResumeOption overrides config fields not persisted in the resume state,
// such as credentials (never written to disk), custom headers, or speed limit.
type ResumeOption func(*DownloadConfig)

// WithAuth supplies HTTP basic-auth credentials for a resumed download.
func WithAuth(username, password string) ResumeOption {
	return func(c *DownloadConfig) { c.Username = username; c.Password = password }
}

// WithHeaders supplies custom request headers for a resumed download.
func WithHeaders(h http.Header) ResumeOption {
	return func(c *DownloadConfig) { c.Headers = h }
}

// WithSpeedLimit supplies a max download rate (bytes/sec) for a resumed download.
func WithSpeedLimit(bytesPerSec int64) ResumeOption {
	return func(c *DownloadConfig) { c.SpeedLimit = bytesPerSec }
}

// WithMirrors supplies additional mirror URLs for a resumed download.
func WithMirrors(mirrors []string) ResumeOption {
	return func(c *DownloadConfig) { c.Mirrors = mirrors }
}

// ResumeDownload loads existing resume state and resumes the download.
func ResumeDownload(ctx context.Context, outputPath string, onProgress ProgressFunc, opts ...ResumeOption) error {
	state, err := loadResumeState(outputPath)
	if err != nil {
		return fmt.Errorf("cannot load resume state: %w", err)
	}
	if state == nil {
		return fmt.Errorf("no resume state found for %s", outputPath)
	}

	cfg := DownloadConfig{
		URL:        state.URL,
		OutputPath: state.OutputFile,
		Workers:    len(state.Chunks),
		Parallel:   true,
	}
	if state.Checksum != nil {
		cfg.Checksum = state.Checksum.Expected
		cfg.ChecksumAlgo = state.Checksum.Algorithm
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	dl := NewDownload(cfg)
	if onProgress != nil {
		dl.OnProgress(onProgress)
	}
	return dl.Start(ctx)
}
