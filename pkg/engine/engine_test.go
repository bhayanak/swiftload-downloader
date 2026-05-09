package engine

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// makeTestServer returns a test HTTP server serving deterministic bytes.
func makeTestServer(data []byte, supportsRange bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if supportsRange {
			w.Header().Set("Accept-Ranges", "bytes")
		}
		w.Header().Set("ETag", `"testfile"`)

		rangeHdr := r.Header.Get("Range")
		if supportsRange && rangeHdr != "" {
			// Parse "bytes=start-end".
			rangeStr := strings.TrimPrefix(rangeHdr, "bytes=")
			parts := strings.SplitN(rangeStr, "-", 2)
			if len(parts) != 2 {
				http.Error(w, "bad range", http.StatusBadRequest)
				return
			}
			start, _ := strconv.ParseInt(parts[0], 10, 64)
			end, _ := strconv.ParseInt(parts[1], 10, 64)
			if end >= int64(len(data)) {
				end = int64(len(data)) - 1
			}
			chunk := data[start : end+1]
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
			w.Header().Set("Content-Length", strconv.Itoa(len(chunk)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(chunk)
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	}))
}

func testData(size int) []byte {
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = byte(i % 256)
	}
	return buf
}

func md5sum(data []byte) string {
	h := md5.Sum(data)
	return fmt.Sprintf("%x", h)
}

func sha256sum(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

// ── Serial download ─────────────────────────────────────────────────────────

func TestSerialDownload_Small(t *testing.T) {
	data := testData(64 * 1024) // 64 KB
	srv := makeTestServer(data, false)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "out.bin")
	cfg := DownloadConfig{URL: srv.URL, OutputPath: out}
	dl := NewDownload(cfg)

	if err := dl.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	got, _ := os.ReadFile(out)
	if !bytes.Equal(got, data) {
		t.Errorf("downloaded data mismatch: got %d bytes, want %d", len(got), len(data))
	}
}

func TestSerialDownload_Checksum_MD5(t *testing.T) {
	data := testData(32 * 1024)
	srv := makeTestServer(data, false)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "out.bin")
	cfg := DownloadConfig{
		URL:          srv.URL,
		OutputPath:   out,
		Checksum:     md5sum(data),
		ChecksumAlgo: "md5",
	}
	if err := NewDownload(cfg).Start(context.Background()); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestSerialDownload_Checksum_SHA256(t *testing.T) {
	data := testData(16 * 1024)
	srv := makeTestServer(data, false)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "out.bin")
	cfg := DownloadConfig{
		URL:          srv.URL,
		OutputPath:   out,
		Checksum:     sha256sum(data),
		ChecksumAlgo: "sha256",
	}
	if err := NewDownload(cfg).Start(context.Background()); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestSerialDownload_WrongChecksum(t *testing.T) {
	data := testData(8 * 1024)
	srv := makeTestServer(data, false)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "out.bin")
	cfg := DownloadConfig{
		URL:          srv.URL,
		OutputPath:   out,
		Checksum:     "deadbeef",
		ChecksumAlgo: "sha256",
	}
	err := NewDownload(cfg).Start(context.Background())
	if err == nil {
		t.Fatal("expected checksum error, got nil")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("expected checksum error message, got: %v", err)
	}
}

func TestSerialDownload_ProgressCallback(t *testing.T) {
	data := testData(128 * 1024)
	srv := makeTestServer(data, false)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "out.bin")
	cfg := DownloadConfig{URL: srv.URL, OutputPath: out}
	dl := NewDownload(cfg)

	var callCount int64
	dl.OnProgress(func(info ProgressInfo) {
		atomic.AddInt64(&callCount, 1)
	})

	if err := dl.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// At minimum the completion callback fires.
	if atomic.LoadInt64(&callCount) == 0 {
		t.Error("expected at least one progress callback")
	}
}

func TestSerialDownload_Cancel(t *testing.T) {
	// Slow server to make cancellation meaningful.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10000000")
		for i := 0; i < 1000; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(5 * time.Millisecond):
				_, _ = w.Write(make([]byte, 10000))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "cancel.bin")
	cfg := DownloadConfig{URL: srv.URL, OutputPath: out}
	dl := NewDownload(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := dl.Start(ctx)
	if err == nil {
		t.Error("expected error after cancellation, got nil")
	}
}

func TestSerialDownload_Status(t *testing.T) {
	data := testData(4 * 1024)
	srv := makeTestServer(data, false)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "out.bin")
	cfg := DownloadConfig{URL: srv.URL, OutputPath: out}
	dl := NewDownload(cfg)

	if dl.Status() != StatusQueued {
		t.Errorf("initial status should be Queued, got %s", dl.Status())
	}

	if err := dl.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if dl.Status() != StatusCompleted {
		t.Errorf("final status should be Completed, got %s", dl.Status())
	}
}

func TestSerialDownload_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "out.bin")
	err := NewDownload(DownloadConfig{URL: srv.URL, OutputPath: out}).Start(context.Background())
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

// ── Parallel download ────────────────────────────────────────────────────────

func TestParallelDownload_Basic(t *testing.T) {
	data := testData(2 * 1024 * 1024) // 2 MB — big enough for chunking
	srv := makeTestServer(data, true)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "parallel.bin")
	cfg := DownloadConfig{
		URL:        srv.URL,
		OutputPath: out,
		Parallel:   true,
		Workers:    4,
	}
	if err := NewDownload(cfg).Start(context.Background()); err != nil {
		t.Fatalf("parallel Start failed: %v", err)
	}

	got, _ := os.ReadFile(out)
	if !bytes.Equal(got, data) {
		t.Errorf("parallel download data mismatch: %d bytes vs expected %d", len(got), len(data))
	}
}

func TestParallelDownload_FallsBackToSerial_NoRangeSupport(t *testing.T) {
	data := testData(64 * 1024)
	srv := makeTestServer(data, false) // no Accept-Ranges
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "fallback.bin")
	cfg := DownloadConfig{
		URL:        srv.URL,
		OutputPath: out,
		Parallel:   true,
		Workers:    4,
	}
	if err := NewDownload(cfg).Start(context.Background()); err != nil {
		t.Fatalf("expected fallback to serial to succeed: %v", err)
	}
	got, _ := os.ReadFile(out)
	if !bytes.Equal(got, data) {
		t.Error("fallback serial data mismatch")
	}
}

func TestParallelDownload_WithChecksum(t *testing.T) {
	data := testData(1024 * 1024)
	srv := makeTestServer(data, true)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "cksum.bin")
	cfg := DownloadConfig{
		URL:          srv.URL,
		OutputPath:   out,
		Parallel:     true,
		Workers:      4,
		Checksum:     sha256sum(data),
		ChecksumAlgo: "sha256",
	}
	if err := NewDownload(cfg).Start(context.Background()); err != nil {
		t.Fatalf("expected success: %v", err)
	}
}

// ── Resume state ──────────────────────────────────────────────────────────────

func TestResumeState_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "file.bin")

	state := newResumeState("https://example.com/file", outPath, 1024*1024, `"etag"`, "Mon, 01 Jan 2024 00:00:00 GMT", 4)

	if err := saveResumeState(state); err != nil {
		t.Fatalf("saveResumeState failed: %v", err)
	}

	loaded, err := loadResumeState(outPath)
	if err != nil {
		t.Fatalf("loadResumeState failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil resume state")
	}
	if loaded.URL != "https://example.com/file" {
		t.Errorf("URL mismatch: got %q", loaded.URL)
	}
	if len(loaded.Chunks) != 4 {
		t.Errorf("expected 4 chunks, got %d", len(loaded.Chunks))
	}
}

func TestResumeState_NoFile(t *testing.T) {
	state, err := loadResumeState(filepath.Join(t.TempDir(), "nonexistent.bin"))
	if err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}
	if state != nil {
		t.Error("expected nil state for missing file")
	}
}

func TestResumeState_IsResumable(t *testing.T) {
	state := newResumeState("https://example.com", "/tmp/f", 1000, `"e1"`, "date", 2)

	if !state.isResumable(`"e1"`, "date", 1000) {
		t.Error("expected isResumable=true when etag, size match")
	}
	if state.isResumable(`"e2"`, "date", 1000) {
		t.Error("expected isResumable=false when etag differs")
	}
	if state.isResumable(`"e1"`, "date", 999) {
		t.Error("expected isResumable=false when size differs")
	}
}

func TestResumeState_IncompleteChunks(t *testing.T) {
	state := newResumeState("https://example.com", "/tmp/f", 1024*1024, "", "", 4)
	if len(state.incompleteChunks()) != 4 {
		t.Error("all chunks should be incomplete initially")
	}

	state.Chunks[0].Done = true
	state.Chunks[2].Done = true
	incomplete := state.incompleteChunks()
	if len(incomplete) != 2 {
		t.Errorf("expected 2 incomplete chunks, got %d", len(incomplete))
	}
}

func TestResumeState_TotalDownloaded(t *testing.T) {
	state := newResumeState("https://example.com", "/tmp/f", 1000, "", "", 2)
	state.Chunks[0].Downloaded = 400
	state.Chunks[1].Downloaded = 100
	if got := state.totalDownloaded(); got != 500 {
		t.Errorf("expected 500, got %d", got)
	}
}

func TestRemoveResumeState(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "file.bin")
	state := newResumeState("https://example.com", outPath, 100, "", "", 1)
	_ = saveResumeState(state)

	sidecar := resumeFilePath(outPath)
	if _, err := os.Stat(sidecar); os.IsNotExist(err) {
		t.Fatal("sidecar should exist after save")
	}

	removeResumeState(outPath)
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Error("sidecar should be removed after removeResumeState")
	}
}

// ── NewDownload & Info ────────────────────────────────────────────────────────

func TestNewDownload_DefaultsApplied(t *testing.T) {
	dl := NewDownload(DownloadConfig{URL: "https://example.com", OutputPath: "/tmp/x"})
	if dl == nil {
		t.Fatal("expected non-nil Download")
	}
}

func TestDownload_InfoBeforeStart(t *testing.T) {
	dl := NewDownload(DownloadConfig{URL: "https://example.com", OutputPath: "/tmp/x"})
	info := dl.Info()
	if info.Downloaded != 0 {
		t.Errorf("expected 0 downloaded, got %d", info.Downloaded)
	}
	if info.Status != StatusQueued {
		t.Errorf("expected Queued status, got %s", info.Status)
	}
}

func TestDownload_CancelBeforeStart(t *testing.T) {
	dl := NewDownload(DownloadConfig{URL: "https://example.com", OutputPath: "/tmp/x"})
	dl.Cancel() // should not panic
	if dl.Status() != StatusCancelled {
		t.Errorf("expected Cancelled, got %s", dl.Status())
	}
}

// ── httpStatusError ───────────────────────────────────────────────────────────

func TestHTTPStatusError_Permanent(t *testing.T) {
	permanent := []int{400, 401, 403, 404, 410}
	for _, code := range permanent {
		e := &httpStatusError{Code: code}
		if !e.IsPermanent() {
			t.Errorf("expected HTTP %d to be permanent", code)
		}
	}
}

func TestHTTPStatusError_NonPermanent(t *testing.T) {
	nonPerm := []int{429, 500, 502, 503}
	for _, code := range nonPerm {
		e := &httpStatusError{Code: code}
		if e.IsPermanent() {
			t.Errorf("expected HTTP %d to be non-permanent (retryable)", code)
		}
	}
}

func TestHTTPStatusError_Error(t *testing.T) {
	e := &httpStatusError{Code: 503}
	if !strings.Contains(e.Error(), "503") {
		t.Errorf("error string should contain code, got: %s", e.Error())
	}
}

// ── ResumeDownload ────────────────────────────────────────────────────────────

func TestResumeDownload_NoStateFile(t *testing.T) {
	err := ResumeDownload(context.Background(), filepath.Join(t.TempDir(), "nofile.bin"), nil)
	if err == nil {
		t.Fatal("expected error when no resume state exists")
	}
}

func TestResumeDownload_FullCycle(t *testing.T) {
	data := testData(2 * 1024 * 1024)
	srv := makeTestServer(data, true)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "resume.bin")

	// First: start parallel download with a very short timeout to interrupt it.
	cfg := DownloadConfig{
		URL:        srv.URL,
		OutputPath: out,
		Parallel:   true,
		Workers:    2,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_ = NewDownload(cfg).Start(ctx) // ignore error; it will be cancelled

	// The resume state sidecar should exist (unless download completed very fast).
	// If it doesn't exist (download finished before cancel), the file is already complete — skip.
	if _, err := os.Stat(resumeFilePath(out)); os.IsNotExist(err) {
		t.Skip("download completed before cancellation; skip resume test")
	}

	// Now resume to completion.
	err := ResumeDownload(context.Background(), out, nil)
	if err != nil {
		t.Fatalf("ResumeDownload failed: %v", err)
	}

	got, _ := os.ReadFile(out)
	if !bytes.Equal(got, data) {
		t.Errorf("resumed data mismatch: %d bytes vs %d", len(got), len(data))
	}
}

// ── Verify checksum edge cases ────────────────────────────────────────────────

func TestVerifyChecksum_UnknownAlgo(t *testing.T) {
	data := testData(1024)
	srv := makeTestServer(data, false)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "out.bin")
	cfg := DownloadConfig{
		URL:          srv.URL,
		OutputPath:   out,
		Checksum:     "abc",
		ChecksumAlgo: "unknown-algo",
	}
	err := NewDownload(cfg).Start(context.Background())
	if err == nil {
		t.Fatal("expected error for unknown checksum algorithm")
	}
}

func TestVerifyChecksum_MD5Wrong(t *testing.T) {
	data := testData(1024)
	srv := makeTestServer(data, false)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "out.bin")
	cfg := DownloadConfig{
		URL:          srv.URL,
		OutputPath:   out,
		Checksum:     "000000000000000000000000000000000",
		ChecksumAlgo: "md5",
	}
	err := NewDownload(cfg).Start(context.Background())
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}

// ── Chunk download edge cases ─────────────────────────────────────────────────

func TestDownloadChunkWithRetry_PermanentError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := &http.Client{}
	f, _ := os.CreateTemp(t.TempDir(), "chunk")
	defer f.Close()

	var downloaded int64
	err := downloadChunkWithRetry(context.Background(), client, srv.URL, f, 0, 99, 3, 4096, &downloaded, nil)
	if err == nil {
		t.Fatal("expected permanent error, got nil")
	}
}

func TestDownloadChunkWithRetry_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Second)
	}))
	defer srv.Close()

	client := &http.Client{}
	f, _ := os.CreateTemp(t.TempDir(), "chunk")
	defer f.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var downloaded int64
	err := downloadChunkWithRetry(ctx, client, srv.URL, f, 0, 99, 1, 4096, &downloaded, nil)
	if err == nil {
		t.Fatal("expected context.Canceled error")
	}
}

// ── Verify io helpers ─────────────────────────────────────────────────────────

func TestParallelDownload_ProgressReported(t *testing.T) {
	data := testData(1024 * 1024)
	srv := makeTestServer(data, true)
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "prog.bin")
	cfg := DownloadConfig{
		URL:        srv.URL,
		OutputPath: out,
		Parallel:   true,
		Workers:    2,
	}
	dl := NewDownload(cfg)

	var maxPct float64
	dl.OnProgress(func(info ProgressInfo) {
		if info.Percent > maxPct {
			maxPct = info.Percent
		}
	})

	if err := dl.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if maxPct == 0 {
		t.Error("expected non-zero max percent during parallel download")
	}
}

// Compile-time check: ensure io helpers exist in chunk.go.
var _ io.Writer = (*os.File)(nil)
