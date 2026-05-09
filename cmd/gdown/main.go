package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bhayanak/swiftload-downloader/pkg/engine"
	"github.com/bhayanak/swiftload-downloader/pkg/util"
	"github.com/spf13/cobra"
)

var version = "2.0.0"

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

	err := dl.Start(ctx)
	fmt.Println() // newline after progress

	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	fmt.Printf("Download complete: %s\n", output)
	return nil
}

func runResume(cmd *cobra.Command, args []string) error {
	outputPath := args[0]

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("Resuming download: %s\n\n", outputPath)

	err := engine.ResumeDownload(ctx, outputPath, cliProgressFunc())
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
