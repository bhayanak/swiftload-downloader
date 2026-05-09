package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
)

// serialDownload performs a single-stream download.
func serialDownload(
	ctx context.Context,
	client *http.Client,
	url string,
	outfile string,
	bufSize int64,
	progress *progressTracker,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %s", resp.Status)
	}

	if s := resp.Header.Get("Content-Length"); s != "" {
		if size, parseErr := strconv.ParseInt(s, 10, 64); parseErr == nil {
			progress.totalSize = size
		}
	}

	file, err := os.Create(outfile)
	if err != nil {
		return fmt.Errorf("cannot create output file: %w", err)
	}
	defer file.Close()

	buf := make([]byte, bufSize)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := file.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write error: %w", werr)
			}
			progress.addBytes(int64(n))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read error: %w", readErr)
		}
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("file sync failed: %w", err)
	}
	return nil
}
