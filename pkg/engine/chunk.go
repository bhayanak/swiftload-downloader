package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

// downloadChunkWithRetry downloads the byte range [start, end] with
// exponential back-off retries. On a partial failure it advances the
// start offset so only the missing tail is re-requested.
func downloadChunkWithRetry(
	ctx context.Context,
	client *http.Client,
	url string,
	file *os.File,
	start, end int64,
	retries int,
	bufSize int64,
	downloaded *int64,
	onRetry func(start, end int64, attempt, maxRetries int, err error, backoff time.Duration),
) error {
	offset := start
	for attempt := 0; ; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		written, err := downloadChunk(ctx, client, url, file, offset, end, bufSize, downloaded)
		if err == nil {
			return nil
		}
		if errors.Is(err, context.Canceled) {
			return err
		}

		// Don't retry on permanent HTTP errors.
		var httpErr *httpStatusError
		if errors.As(err, &httpErr) && httpErr.IsPermanent() {
			return err
		}

		if attempt >= retries {
			return fmt.Errorf("chunk bytes=%d-%d failed after %d attempt(s): %w", start, end, attempt+1, err)
		}

		offset += written

		backoff := time.Duration(math.Pow(2, float64(attempt+1))) * 200 * time.Millisecond
		if onRetry != nil {
			onRetry(start, end, attempt+1, retries, err, backoff)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
	}
}

// downloadChunk fetches [start, end] bytes from the server and writes them
// to file at the correct offset. Returns bytes written and any error.
func downloadChunk(
	ctx context.Context,
	client *http.Client,
	url string,
	file *os.File,
	start, end int64,
	bufSize int64,
	downloaded *int64,
) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		return 0, &httpStatusError{Code: resp.StatusCode}
	}

	buf := make([]byte, bufSize)
	offset := start
	var written int64

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := file.WriteAt(buf[:n], offset); werr != nil {
				return written, fmt.Errorf("write error at offset %d: %w", offset, werr)
			}
			offset += int64(n)
			written += int64(n)
			atomic.AddInt64(downloaded, int64(n))
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}

// httpStatusError represents an HTTP error with a status code.
type httpStatusError struct {
	Code int
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("unexpected HTTP status %d for Range request", e.Code)
}

// IsPermanent returns true for HTTP status codes that should not be retried.
func (e *httpStatusError) IsPermanent() bool {
	return e.Code == 400 || e.Code == 401 || e.Code == 403 || e.Code == 404 || e.Code == 410
}
