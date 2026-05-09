package gui

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/bhayanak/gdown/pkg/engine"
	"github.com/bhayanak/gdown/pkg/util"
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

	cfg    engine.DownloadConfig
	dl     *engine.Download
	cancel context.CancelFunc
	status rowStatus
	mu     sync.Mutex
	mw     *MainWindow
}

// NewDownloadRow creates a new download row and starts the download.
func NewDownloadRow(mw *MainWindow, cfg engine.DownloadConfig) *DownloadRow {
	row := &DownloadRow{
		mw:          mw,
		cfg:         cfg,
		nameLabel:   widget.NewLabel(filenameFromPath(cfg.OutputPath)),
		sizeLabel:   widget.NewLabel("—"),
		progressBar: widget.NewProgressBar(),
		speedLabel:  widget.NewLabel("—"),
		etaLabel:    widget.NewLabel("—"),
		status:      rowStatusQueued,
	}

	row.pauseBtn = widget.NewButton("⏸", func() {
		row.togglePause()
	})
	row.restartBtn = widget.NewButton("↻", func() {
		row.Restart()
	})
	row.restartBtn.Hide()
	row.revealBtn = widget.NewButton("📂", func() {
		revealInFinder(cfg.OutputPath)
	})
	row.revealBtn.Hide()
	row.cancelBtn = widget.NewButton("✕", func() {
		row.Cancel()
		mw.RemoveDownloadRow(row)
	})

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
				errMsg := truncate(err.Error(), 30)
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
			r.status = rowStatusCompleted
			fyne.Do(func() {
				r.speedLabel.SetText("Done")
				r.etaLabel.SetText("—")
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
		}
		r.speedLabel.SetText(util.FormatSpeed(info.Speed))
		if info.ETA > 0 {
			r.etaLabel.SetText(util.FormatDuration(info.ETA))
		} else {
			r.etaLabel.SetText("—")
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
	fyne.Do(func() {
		r.pauseBtn.SetText("▶")
		r.speedLabel.SetText("Paused")
		r.mw.refreshStatusBar()
	})
}

// Resume re-starts the download from where it left off using saved resume state.
func (r *DownloadRow) Resume() {
	if r.status != rowStatusPaused {
		return
	}
	r.status = rowStatusDownloading
	fyne.Do(func() {
		r.pauseBtn.SetText("⏸")
	})

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
			log.Printf("[resume] FAILED %s: %v, will allow restart", outputPath, err)
			if r.status != rowStatusCancelled && r.status != rowStatusPaused {
				r.status = rowStatusFailed
				errMsg := truncate(err.Error(), 30)
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
			r.status = rowStatusCompleted
			fyne.Do(func() {
				r.speedLabel.SetText("Done")
				r.etaLabel.SetText("—")
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

// fmtf is a convenience alias.
func fmtf(format string, a ...any) string {
	return fmt.Sprintf(format, a...)
}
