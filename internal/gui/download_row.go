package gui

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bhayanak/swiftload-downloader/pkg/engine"
	"github.com/bhayanak/swiftload-downloader/pkg/util"
)

type rowStatus int

const (
	rowStatusQueued rowStatus = iota
	rowStatusDownloading
	rowStatusPaused
	rowStatusCompleted
	rowStatusFailed
	rowStatusCancelled
)

// DownloadRow represents a single download in the GUI list.
type DownloadRow struct {
	container *fyne.Container

	nameLabel     *widget.Label
	sizeLabel     *widget.Label
	progressBar   *widget.ProgressBar
	speedLabel    *widget.Label
	etaLabel      *widget.Label
	pauseBtn      *widget.Button
	restartBtn    *widget.Button
	cancelBtn     *widget.Button
	revealBtn     *widget.Button

	cfg       engine.DownloadConfig
	dl        *engine.Download
	cancel    context.CancelFunc
	status    rowStatus
	mu        sync.Mutex
	mw        *MainWindow
	historyID string
	startedAt time.Time
}

// NewDownloadRow creates a new download row and starts the download.
func NewDownloadRow(mw *MainWindow, cfg engine.DownloadConfig) *DownloadRow {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	row := &DownloadRow{
		mw:          mw,
		cfg:         cfg,
		nameLabel:   widget.NewLabel(filenameFromPath(cfg.OutputPath)),
		sizeLabel:   widget.NewLabel("—"),
		progressBar: widget.NewProgressBar(),
		speedLabel:  widget.NewLabel("—"),
		etaLabel:    widget.NewLabel("—"),
		status:      rowStatusQueued,
		historyID:   id,
	}

	// Persist to history.
	mw.history.Add(HistoryEntry{
		ID:         id,
		URL:        cfg.URL,
		OutputPath: cfg.OutputPath,
		Status:     "downloading",
		AddedAt:    time.Now(),
		Workers:    cfg.Workers,
		Parallel:   cfg.Parallel,
	})

	row.pauseBtn = widget.NewButton("⏸", func() {
		row.togglePause()
	})
	row.restartBtn = widget.NewButton("↻", func() {
		row.Restart()
	})
	row.restartBtn.Hide()
	row.revealBtn = widget.NewButton("📂", func() {
		row.revealOrRedownload()
	})
	row.revealBtn.Hide()
	row.cancelBtn = widget.NewButton("✕", func() {
		ShowDeleteDialog(mw, row)
	})

	row.sizeLabel.Alignment = fyne.TextAlignCenter
	row.speedLabel.Alignment = fyne.TextAlignCenter
	row.etaLabel.Alignment = fyne.TextAlignCenter
	row.nameLabel.Truncation = fyne.TextTruncateEllipsis

	actions := container.NewHBox(row.pauseBtn, row.restartBtn, row.revealBtn, row.cancelBtn)

	row.container = container.NewGridWithColumns(6,
		row.nameLabel,
		row.sizeLabel,
		row.progressBar,
		row.speedLabel,
		row.etaLabel,
		actions,
	)

	row.startDownload()
	return row
}

// startDownload begins (or restarts) the download.
func (r *DownloadRow) startDownload() {
	dl := engine.NewDownload(r.cfg)
	dl.OnProgress(func(info engine.ProgressInfo) {
		r.updateFromProgress(info)
	})
	r.dl = dl
	r.startedAt = time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.status = rowStatusDownloading

	log.Printf("[download] starting: %s -> %s (parallel=%v, workers=%d)",
		r.cfg.URL, r.cfg.OutputPath, r.cfg.Parallel, r.cfg.Workers)

	go func() {
		err := dl.Start(ctx)
		r.mu.Lock()
		defer r.mu.Unlock()
		if err != nil {
			log.Printf("[download] FAILED %s: %v", r.cfg.OutputPath, err)
			if r.status != rowStatusCancelled && r.status != rowStatusPaused {
				r.status = rowStatusFailed
				r.mw.history.Update(r.historyID, func(e *HistoryEntry) {
					e.Status = "failed"
				})
				errMsg := truncate(err.Error(), 40)
				fyne.Do(func() {
					r.speedLabel.SetText("Failed")
					r.etaLabel.SetText(errMsg)
					r.pauseBtn.Hide()
					r.restartBtn.Show()
					r.mw.refreshStatusBar()
				})
			}
		} else {
			log.Printf("[download] DONE %s", r.cfg.OutputPath)
			elapsed := time.Since(r.startedAt)
			finalInfo := dl.Info()
			r.status = rowStatusCompleted
			r.mw.history.Update(r.historyID, func(e *HistoryEntry) {
				e.Status = "completed"
				e.FinishedAt = time.Now()
				if finalInfo.TotalSize > 0 {
					e.Downloaded = finalInfo.TotalSize
					e.TotalSize = finalInfo.TotalSize
				} else {
					e.Downloaded = finalInfo.Downloaded
					e.TotalSize = finalInfo.Downloaded
				}
			})
			fyne.Do(func() {
				// Show final size (use TotalSize if known, otherwise Downloaded).
				finalSize := finalInfo.TotalSize
				if finalSize <= 0 {
					finalSize = finalInfo.Downloaded
				}
				if finalSize > 0 {
					r.sizeLabel.SetText(util.FormatBytes(finalSize))
				}
				r.speedLabel.SetText("Done")
				r.etaLabel.SetText(util.FormatDuration(elapsed))
				r.progressBar.SetValue(1.0)
				r.pauseBtn.Hide()
				r.revealBtn.Show()
				r.mw.refreshStatusBar()
			})
		}
	}()
}

func (r *DownloadRow) updateFromProgress(info engine.ProgressInfo) {
	fyne.Do(func() {
		if info.TotalSize > 0 {
			r.sizeLabel.SetText(util.FormatBytes(info.TotalSize))
			r.progressBar.SetValue(info.Percent / 100.0)
			r.mw.history.Update(r.historyID, func(e *HistoryEntry) {
				e.TotalSize = info.TotalSize
				e.Downloaded = info.Downloaded
			})
		} else if info.Downloaded > 0 {
			// Unknown total size: show downloaded so far.
			r.sizeLabel.SetText(util.FormatBytes(info.Downloaded))
			r.mw.history.Update(r.historyID, func(e *HistoryEntry) {
				e.Downloaded = info.Downloaded
			})
		}
		r.speedLabel.SetText(util.FormatSpeed(info.Speed))
		if info.ETA > 0 {
			r.etaLabel.SetText(util.FormatDuration(info.ETA))
		} else if !r.startedAt.IsZero() {
			r.etaLabel.SetText(util.FormatDuration(time.Since(r.startedAt)))
		}
	})
}

func (r *DownloadRow) togglePause() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.status == rowStatusDownloading {
		r.Pause()
	} else if r.status == rowStatusPaused {
		r.Resume()
	}
}

// Pause cancels the current download context (engine saves resume state).
func (r *DownloadRow) Pause() {
	if r.status != rowStatusDownloading {
		return
	}
	r.status = rowStatusPaused
	if r.cancel != nil {
		r.cancel()
	}
	r.mw.history.Update(r.historyID, func(e *HistoryEntry) {
		e.Status = "paused"
	})
	fyne.Do(func() {
		r.pauseBtn.SetText("▶")
		r.speedLabel.SetText("Paused")
		r.mw.refreshStatusBar()
	})
}

// Resume re-starts the download from where it left off using saved resume state.
// If no resume state exists, automatically falls back to a fresh download.
func (r *DownloadRow) Resume() {
	if r.status != rowStatusPaused {
		return
	}
	r.status = rowStatusDownloading
	r.mw.history.Update(r.historyID, func(e *HistoryEntry) {
		e.Status = "downloading"
	})
	fyne.Do(func() {
		r.pauseBtn.SetText("⏸")
		r.speedLabel.SetText("—")
		r.etaLabel.SetText("—")
	})

	r.startedAt = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	outputPath := r.cfg.OutputPath

	go func() {
		err := engine.ResumeDownload(ctx, outputPath, func(info engine.ProgressInfo) {
			r.updateFromProgress(info)
		})
		r.mu.Lock()
		defer r.mu.Unlock()
		if err != nil {
			// If no resume state found, fall back to fresh download automatically.
			if strings.Contains(err.Error(), "no resume state") {
				log.Printf("[resume] no resume state for %s, restarting fresh", outputPath)
				r.mu.Unlock()
				r.Restart()
				r.mu.Lock()
				return
			}
			log.Printf("[resume] FAILED %s: %v", outputPath, err)
			if r.status != rowStatusCancelled && r.status != rowStatusPaused {
				r.status = rowStatusFailed
				r.mw.history.Update(r.historyID, func(e *HistoryEntry) {
					e.Status = "failed"
				})
				errMsg := truncate(err.Error(), 40)
				fyne.Do(func() {
					r.speedLabel.SetText("Failed")
					r.etaLabel.SetText(errMsg)
					r.pauseBtn.Hide()
					r.restartBtn.Show()
					r.mw.refreshStatusBar()
				})
			}
		} else {
			log.Printf("[resume] DONE %s", outputPath)
			elapsed := time.Since(r.startedAt)
			r.status = rowStatusCompleted
			r.mw.history.Update(r.historyID, func(e *HistoryEntry) {
				e.Status = "completed"
				e.FinishedAt = time.Now()
				if e.TotalSize > 0 {
					e.Downloaded = e.TotalSize
				}
			})
			fyne.Do(func() {
				r.speedLabel.SetText("Done")
				r.etaLabel.SetText(util.FormatDuration(elapsed))
				r.progressBar.SetValue(1.0)
				r.pauseBtn.Hide()
				r.revealBtn.Show()
				r.mw.refreshStatusBar()
			})
		}
	}()
}

// Restart re-downloads from scratch (when resume fails or download failed).
func (r *DownloadRow) Restart() {
	r.mu.Lock()
	defer r.mu.Unlock()
	log.Printf("[restart] restarting download: %s", r.cfg.OutputPath)
	r.status = rowStatusDownloading
	fyne.Do(func() {
		r.progressBar.SetValue(0)
		r.speedLabel.SetText("—")
		r.etaLabel.SetText("—")
		r.sizeLabel.SetText("—")
		r.restartBtn.Hide()
		r.revealBtn.Hide()
		r.pauseBtn.SetText("⏸")
		r.pauseBtn.Show()
	})
	r.startDownload()
}

// Cancel stops the download permanently.
func (r *DownloadRow) Cancel() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = rowStatusCancelled
	if r.cancel != nil {
		r.cancel()
	}
}

// truncate shortens a string to maxLen chars, adding "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

// revealInFinder opens the file's parent directory in the system file manager.
// If the file doesn't exist, shows a dialog offering to redownload.
func (r *DownloadRow) revealOrRedownload() {
	path := r.cfg.OutputPath
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		fyne.Do(func() {
			dialog.ShowConfirm("File Not Found",
				fmt.Sprintf("The file \"%s\" appears to have been deleted.\nWould you like to redownload it?", filenameFromPath(path)),
				func(yes bool) {
					if yes {
						r.Restart()
					}
				}, r.mw.window)
		})
		return
	}
	revealInFinder(path)
}

// revealInFinder opens the file's parent directory in the system file manager.
func revealInFinder(path string) {
	dir := filepath.Dir(path)
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", "-R", path).Start()
	case "windows":
		_ = exec.Command("explorer", "/select,", path).Start()
	default:
		_ = exec.Command("xdg-open", dir).Start()
	}
}

// filenameFromPath extracts the filename from a path.
func filenameFromPath(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}

// RestoreDownloadRow creates a row from a persisted history entry (no active download).
func RestoreDownloadRow(mw *MainWindow, entry HistoryEntry) *DownloadRow {
	row := &DownloadRow{
		mw:        mw,
		historyID: entry.ID,
		cfg: engine.DownloadConfig{
			URL:        entry.URL,
			OutputPath: entry.OutputPath,
			Workers:    entry.Workers,
			Parallel:   entry.Parallel,
		},
		nameLabel:   widget.NewLabel(filenameFromPath(entry.OutputPath)),
		sizeLabel:   widget.NewLabel("—"),
		progressBar: widget.NewProgressBar(),
		speedLabel:  widget.NewLabel("—"),
		etaLabel:    widget.NewLabel("—"),
	}

	// Set visual state based on persisted status.
	switch entry.Status {
	case "completed":
		row.status = rowStatusCompleted
		row.speedLabel.SetText("Done")
		if entry.TotalSize > 0 {
			row.sizeLabel.SetText(util.FormatBytes(entry.TotalSize))
			row.progressBar.SetValue(1.0)
		} else if entry.Downloaded > 0 {
			row.sizeLabel.SetText(util.FormatBytes(entry.Downloaded))
			row.progressBar.SetValue(1.0)
		}
		if !entry.FinishedAt.IsZero() && !entry.AddedAt.IsZero() {
			row.etaLabel.SetText(util.FormatDuration(entry.FinishedAt.Sub(entry.AddedAt)))
		}
	case "failed":
		row.status = rowStatusFailed
		row.speedLabel.SetText("Failed")
		if entry.TotalSize > 0 {
			row.sizeLabel.SetText(util.FormatBytes(entry.TotalSize))
			pct := float64(entry.Downloaded) / float64(entry.TotalSize)
			row.progressBar.SetValue(pct)
		}
	case "paused", "downloading":
		// Treat interrupted downloads as paused.
		row.status = rowStatusPaused
		row.speedLabel.SetText("Paused")
		if entry.TotalSize > 0 {
			row.sizeLabel.SetText(util.FormatBytes(entry.TotalSize))
			pct := float64(entry.Downloaded) / float64(entry.TotalSize)
			row.progressBar.SetValue(pct)
		}
	default:
		row.status = rowStatusCancelled
		row.speedLabel.SetText("Cancelled")
	}

	// Buttons.
	row.pauseBtn = widget.NewButton("▶", func() {
		row.togglePause()
	})
	row.restartBtn = widget.NewButton("↻", func() {
		row.Restart()
	})
	row.revealBtn = widget.NewButton("📂", func() {
		row.revealOrRedownload()
	})
	row.cancelBtn = widget.NewButton("✕", func() {
		ShowDeleteDialog(mw, row)
	})

	// Show/hide buttons based on status.
	switch row.status {
	case rowStatusCompleted:
		row.pauseBtn.Hide()
		row.restartBtn.Hide()
	case rowStatusFailed:
		row.pauseBtn.Hide()
		row.revealBtn.Hide()
	case rowStatusPaused:
		row.pauseBtn.SetText("▶")
		row.restartBtn.Hide()
		row.revealBtn.Hide()
	default:
		row.pauseBtn.Hide()
		row.restartBtn.Hide()
		row.revealBtn.Hide()
	}

	actions := container.NewHBox(row.pauseBtn, row.restartBtn, row.revealBtn, row.cancelBtn)

	row.sizeLabel.Alignment = fyne.TextAlignCenter
	row.speedLabel.Alignment = fyne.TextAlignCenter
	row.etaLabel.Alignment = fyne.TextAlignCenter
	row.nameLabel.Truncation = fyne.TextTruncateEllipsis

	row.container = container.NewGridWithColumns(6,
		row.nameLabel,
		row.sizeLabel,
		row.progressBar,
		row.speedLabel,
		row.etaLabel,
		actions,
	)

	return row
}

// fmtf is a convenience alias.
func fmtf(format string, a ...any) string {
	return fmt.Sprintf(format, a...)
}
