package gui

import (
	"context"
	"fmt"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/yadava/gdown/pkg/engine"
	"github.com/yadava/gdown/pkg/util"
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
	cancelBtn     *widget.Button

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
	row.cancelBtn = widget.NewButton("✕", func() {
		row.Cancel()
		mw.RemoveDownloadRow(row)
	})

	actions := container.NewHBox(row.pauseBtn, row.cancelBtn)

	row.container = container.NewGridWithColumns(6,
		row.nameLabel,
		row.sizeLabel,
		row.progressBar,
		row.speedLabel,
		row.etaLabel,
		actions,
	)

	dl := engine.NewDownload(cfg)
	dl.OnProgress(func(info engine.ProgressInfo) {
		row.updateFromProgress(info)
	})
	row.dl = dl

	// Start download in background.
	ctx, cancel := context.WithCancel(context.Background())
	row.cancel = cancel
	row.status = rowStatusDownloading

	go func() {
		err := dl.Start(ctx)
		row.mu.Lock()
		defer row.mu.Unlock()
		if err != nil {
			if row.status != rowStatusCancelled && row.status != rowStatusPaused {
				row.status = rowStatusFailed
				fyne.Do(func() {
					row.speedLabel.SetText("Failed")
					row.etaLabel.SetText("—")
				})
			}
		} else {
			row.status = rowStatusCompleted
			fyne.Do(func() {
				row.speedLabel.SetText("Done")
				row.etaLabel.SetText("—")
				row.progressBar.SetValue(1.0)
				row.pauseBtn.Disable()
			})
		}
	}()

	return row
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
	r.pauseBtn.SetText("▶")
	r.speedLabel.SetText("Paused")
}

// Resume re-starts the download from where it left off using saved resume state.
func (r *DownloadRow) Resume() {
	if r.status != rowStatusPaused {
		return
	}
	r.status = rowStatusDownloading
	r.pauseBtn.SetText("⏸")

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
			if r.status != rowStatusCancelled && r.status != rowStatusPaused {
				r.status = rowStatusFailed
				fyne.Do(func() {
					r.speedLabel.SetText("Failed")
				})
			}
		} else {
			r.status = rowStatusCompleted
			fyne.Do(func() {
				r.speedLabel.SetText("Done")
				r.progressBar.SetValue(1.0)
				r.pauseBtn.Disable()
			})
		}
	}()
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
