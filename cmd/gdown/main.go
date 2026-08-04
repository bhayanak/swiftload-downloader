package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bhayanak/swiftload-downloader/pkg/engine"
	"github.com/bhayanak/swiftload-downloader/pkg/util"
	"github.com/spf13/cobra"
)

var version = "2.4.0"

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "gdown",
	Short: "gdown — High-performance parallel file downloader",
	Long: `gdown is a fast, reliable download manager with parallel chunked
downloading, resume capability, and retry support.

Install via: go install github.com/bhayanak/swiftload-downloader/cmd/gdown@latest`,
}

func init() {
	rootCmd.AddCommand(downloadCmd)
	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(versionCmd)

	// Download command flags.
	downloadCmd.Flags().StringP("output", "o", "", "Destination file path (auto-detected from URL if omitted)")
	downloadCmd.Flags().BoolP("parallel", "p", false, "Enable parallel chunked downloading")
	downloadCmd.Flags().IntP("workers", "w", engine.DefaultWorkers, "Number of concurrent chunk workers")
	downloadCmd.Flags().IntP("retries", "r", engine.DefaultRetries, "Max retry attempts per chunk")
	downloadCmd.Flags().Int("bufsize", engine.DefaultBufSizeMB, "Per-worker read-buffer size in MB")
	downloadCmd.Flags().Bool("proxy", false, "Use HTTP_PROXY/HTTPS_PROXY/NO_PROXY from environment")
	downloadCmd.Flags().String("checksum", "", "Expected checksum hash for verification")
	downloadCmd.Flags().String("checksum-algo", "sha256", "Checksum algorithm: md5, sha256")
	downloadCmd.Flags().StringP("user", "u", "", "HTTP basic-auth username")
	downloadCmd.Flags().String("password", "", "HTTP basic-auth password")
	downloadCmd.Flags().StringArrayP("header", "H", nil, "Custom request header 'Key: Value' (repeatable)")
	downloadCmd.Flags().String("limit", "", "Max download speed, e.g. 500k, 2m (0=unlimited)")
	downloadCmd.Flags().StringArray("mirror", nil, "Additional mirror URL serving the same file (repeatable)")

	// Resume command flags (credentials are not persisted, so re-supply them).
	resumeCmd.Flags().StringP("user", "u", "", "HTTP basic-auth username")
	resumeCmd.Flags().String("password", "", "HTTP basic-auth password")
	resumeCmd.Flags().StringArrayP("header", "H", nil, "Custom request header 'Key: Value' (repeatable)")
	resumeCmd.Flags().String("limit", "", "Max download speed, e.g. 500k, 2m (0=unlimited)")
	resumeCmd.Flags().StringArray("mirror", nil, "Additional mirror URL serving the same file (repeatable)")
}

var downloadCmd = &cobra.Command{
	Use:   "download <url>",
	Short: "Download a file from a URL",
	Long: `Download a file from an HTTP(S) URL with optional parallel chunked
downloading, automatic retry, and resume support.

Examples:
  gdown download https://example.com/file.iso
  gdown download https://example.com/file.iso -o file.iso -p
  gdown download https://example.com/file.iso -p -w 64 --bufsize 8`,
	Args: cobra.ExactArgs(1),
	RunE: runDownload,
}

var resumeCmd = &cobra.Command{
	Use:   "resume <file>",
	Short: "Resume an interrupted download",
	Long: `Resume a previously interrupted download using the saved
.gdown.json metadata file.

Examples:
  gdown resume file.iso
  gdown resume /path/to/output.bin`,
	Args: cobra.ExactArgs(1),
	RunE: runResume,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("gdown version %s\n", version)
	},
}

func runDownload(cmd *cobra.Command, args []string) error {
	url := args[0]

	output, _ := cmd.Flags().GetString("output")
	if output == "" {
		output = filenameFromURL(url)
	}

	parallel, _ := cmd.Flags().GetBool("parallel")
	workers, _ := cmd.Flags().GetInt("workers")
	retries, _ := cmd.Flags().GetInt("retries")
	bufMB, _ := cmd.Flags().GetInt("bufsize")
	useProxy, _ := cmd.Flags().GetBool("proxy")
	checksum, _ := cmd.Flags().GetString("checksum")
	checksumAlgo, _ := cmd.Flags().GetString("checksum-algo")
	username, _ := cmd.Flags().GetString("user")
	password, _ := cmd.Flags().GetString("password")
	headerList, _ := cmd.Flags().GetStringArray("header")
	limitStr, _ := cmd.Flags().GetString("limit")
	mirrors, _ := cmd.Flags().GetStringArray("mirror")

	headers, err := parseHeaders(headerList)
	if err != nil {
		return err
	}
	speedLimit, err := parseSize(limitStr)
	if err != nil {
		return fmt.Errorf("invalid --limit: %w", err)
	}

	cfg := engine.DownloadConfig{
		URL:          url,
		OutputPath:   output,
		Workers:      workers,
		Retries:      retries,
		BufSize:      int64(bufMB) * 1024 * 1024,
		UseProxy:     useProxy,
		Parallel:     parallel,
		Checksum:     checksum,
		ChecksumAlgo: checksumAlgo,
		Username:     username,
		Password:     password,
		Headers:      headers,
		SpeedLimit:   speedLimit,
		Mirrors:      mirrors,
	}

	dl := engine.NewDownload(cfg)
	dl.OnProgress(cliProgressFunc())

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("Downloading: %s\n", url)
	fmt.Printf("Output:      %s\n", output)
	if parallel {
		fmt.Printf("Mode:        parallel (%d workers)\n", workers)
	} else {
		fmt.Printf("Mode:        serial\n")
	}
	fmt.Println()

	err = dl.Start(ctx)
	fmt.Println() // newline after progress

	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	fmt.Printf("Download complete: %s\n", output)
	return nil
}

func runResume(cmd *cobra.Command, args []string) error {
	outputPath := args[0]

	username, _ := cmd.Flags().GetString("user")
	password, _ := cmd.Flags().GetString("password")
	headerList, _ := cmd.Flags().GetStringArray("header")
	limitStr, _ := cmd.Flags().GetString("limit")
	mirrors, _ := cmd.Flags().GetStringArray("mirror")

	headers, err := parseHeaders(headerList)
	if err != nil {
		return err
	}
	speedLimit, err := parseSize(limitStr)
	if err != nil {
		return fmt.Errorf("invalid --limit: %w", err)
	}

	var opts []engine.ResumeOption
	if username != "" {
		opts = append(opts, engine.WithAuth(username, password))
	}
	if headers != nil {
		opts = append(opts, engine.WithHeaders(headers))
	}
	if speedLimit > 0 {
		opts = append(opts, engine.WithSpeedLimit(speedLimit))
	}
	if len(mirrors) > 0 {
		opts = append(opts, engine.WithMirrors(mirrors))
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("Resuming download: %s\n\n", outputPath)

	err = engine.ResumeDownload(ctx, outputPath, cliProgressFunc(), opts...)
	fmt.Println()

	if err != nil {
		return fmt.Errorf("resume failed: %w", err)
	}
	fmt.Printf("Download complete: %s\n", outputPath)
	return nil
}

// cliProgressFunc returns a ProgressFunc that prints progress to stdout.
func cliProgressFunc() engine.ProgressFunc {
	return func(info engine.ProgressInfo) {
		speed := info.Speed / 1024.0 / 1024.0 // MB/s
		if info.TotalSize > 0 {
			var eta string
			if info.ETA > 0 {
				eta = util.FormatDuration(info.ETA)
			} else {
				eta = "—"
			}
			fmt.Printf("\r%s / %s (%.1f%%) | %.2f MB/s | ETA: %s     ",
				util.FormatBytes(info.Downloaded),
				util.FormatBytes(info.TotalSize),
				info.Percent,
				speed,
				eta,
			)
		} else {
			fmt.Printf("\r%s | %.2f MB/s",
				util.FormatBytes(info.Downloaded),
				speed,
			)
		}
	}
}

// filenameFromURL extracts a filename from the URL path.
func filenameFromURL(rawURL string) string {
	// Remove query string and fragment.
	u := rawURL
	if idx := strings.IndexAny(u, "?#"); idx != -1 {
		u = u[:idx]
	}
	// Get last path segment.
	if idx := strings.LastIndex(u, "/"); idx != -1 {
		u = u[idx+1:]
	}
	if u == "" {
		u = "download_" + time.Now().Format("20060102_150405")
	}
	return u
}

// parseHeaders turns a list of "Key: Value" strings into an http.Header.
func parseHeaders(list []string) (http.Header, error) {
	if len(list) == 0 {
		return nil, nil
	}
	h := make(http.Header)
	for _, item := range list {
		idx := strings.Index(item, ":")
		if idx < 0 {
			return nil, fmt.Errorf("invalid header %q, expected 'Key: Value'", item)
		}
		key := strings.TrimSpace(item[:idx])
		val := strings.TrimSpace(item[idx+1:])
		if key == "" {
			return nil, fmt.Errorf("invalid header %q, empty key", item)
		}
		h.Add(key, val)
	}
	return h, nil
}

// parseSize parses a human size like "500k", "2m", "1g" into bytes. "" or "0" => 0.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "0" {
		return 0, nil
	}
	mult := int64(1)
	switch s[len(s)-1] {
	case 'k':
		mult = 1024
		s = s[:len(s)-1]
	case 'm':
		mult = 1024 * 1024
		s = s[:len(s)-1]
	case 'g':
		mult = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, fmt.Errorf("cannot parse size %q", s)
	}
	return int64(n * float64(mult)), nil
}
