package gui

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/bhayanak/swiftload-downloader/pkg/engine"
	"github.com/bhayanak/swiftload-downloader/pkg/util"
)

// MainWindow is the primary application window with the download list.
type MainWindow struct {
	window       fyne.Window
	app          fyne.App
	downloads    []*DownloadRow
	list         *fyne.Container
	mu           sync.Mutex
	statusBar    *widget.Label
	updateBanner *fyne.Container
	settings     AppSettings
	history      *HistoryStore

	clipStop chan struct{}
	clipLast string
}

// NewMainWindow creates the main application window.
func NewMainWindow(a fyne.App) *MainWindow {
	w := a.NewWindow("Swiftload Downloader")
	w.Resize(fyne.NewSize(900, 500))

	mw := &MainWindow{
		window:    w,
		app:       a,
		statusBar: widget.NewLabel("Ready — No active downloads"),
		settings:  LoadSettings(a),
		history:   NewHistoryStore(a),
	}

	mw.list = container.NewVBox()
	mw.updateBanner = container.NewHBox() // initially empty

	// Toolbar.
	toolbar := container.NewHBox(
		func() *widget.Button {
			b := widget.NewButtonWithIcon("Add URL", theme.ContentAddIcon(), func() { ShowAddDialog(mw) })
			b.Importance = widget.HighImportance
			return b
		}(),
		widget.NewButtonWithIcon("Pause All", theme.MediaPauseIcon(), func() {
			mw.pauseAll()
		}),
		widget.NewButtonWithIcon("Resume All", theme.MediaPlayIcon(), func() {
			mw.resumeAll()
		}),
		widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), func() {
			ShowSettingsDialog(mw)
		}),
		layout.NewSpacer(),
		widget.NewButtonWithIcon("Check Update", theme.ViewRefreshIcon(), func() {
			mw.checkForUpdateManual()
		}),
		widget.NewButtonWithIcon("About", theme.InfoIcon(), func() {
			showAboutDialog(mw)
		}),
	)

	// Header row (padded to align with the padded download cards below).
	header := container.NewPadded(container.NewGridWithColumns(6,
		widget.NewLabelWithStyle("Filename", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Size", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Progress", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Speed", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("ETA", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Actions", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
	))

	scrollable := container.NewVScroll(mw.list)
	scrollable.SetMinSize(fyne.NewSize(880, 350))

	content := container.NewBorder(
		container.NewVBox(mw.updateBanner, toolbar, header), // top
		mw.statusBar, // bottom
		nil, nil,     // left, right
		scrollable,   // center
	)

	w.SetContent(content)
	w.SetCloseIntercept(func() {
		mw.cancelAll()
		mw.history.Stop()
		w.Close()
	})

	// Restore previous downloads from history.
	mw.restoreHistory()

	// Start clipboard monitor if enabled.
	mw.applyClipboardSetting()

	// Background update check (once per 24h).
	go mw.checkForUpdateSilent()

	return mw
}

// Show displays the main window.
func (mw *MainWindow) Show() {
	mw.window.Show()
}

// AddDownloadRow adds a new download row to the list and schedules it.
func (mw *MainWindow) AddDownloadRow(row *DownloadRow) {
	mw.mu.Lock()
	mw.downloads = append(mw.downloads, row)
	mw.list.Add(row.container)
	mw.updateStatusBar()
	mw.mu.Unlock()
	mw.schedule()
}

// schedule starts queued downloads while respecting the max-concurrent limit.
// It must NOT be called while holding mw.mu.
func (mw *MainWindow) schedule() {
	max := mw.settings.MaxConcurrent
	if max <= 0 {
		max = 3
	}

	mw.mu.Lock()
	active := 0
	for _, r := range mw.downloads {
		if r.status == rowStatusDownloading {
			active++
		}
	}
	var toStart []*DownloadRow
	for _, r := range mw.downloads {
		if active >= max {
			break
		}
		if r.status == rowStatusQueued {
			r.status = rowStatusDownloading // reserve the slot before unlocking
			toStart = append(toStart, r)
			active++
		}
	}
	mw.mu.Unlock()

	for _, r := range toStart {
		r.start()
	}
}

// RemoveDownloadRow removes a download row from the list.
func (mw *MainWindow) RemoveDownloadRow(row *DownloadRow) {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	for i, r := range mw.downloads {
		if r == row {
			mw.downloads = append(mw.downloads[:i], mw.downloads[i+1:]...)
			mw.list.Remove(row.container)
			break
		}
	}
	mw.updateStatusBar()
}

func (mw *MainWindow) pauseAll() {
	mw.mu.Lock()
	rows := make([]*DownloadRow, len(mw.downloads))
	copy(rows, mw.downloads)
	mw.mu.Unlock()
	for _, row := range rows {
		row.Pause()
	}
}

func (mw *MainWindow) resumeAll() {
	mw.mu.Lock()
	rows := make([]*DownloadRow, len(mw.downloads))
	copy(rows, mw.downloads)
	mw.mu.Unlock()
	for _, row := range rows {
		if row.status == rowStatusPaused {
			row.enqueue()
		}
	}
	mw.schedule()
}

func (mw *MainWindow) cancelAll() {
	mw.mu.Lock()
	rows := make([]*DownloadRow, len(mw.downloads))
	copy(rows, mw.downloads)
	mw.mu.Unlock()
	for _, row := range rows {
		row.Cancel()
	}
}

// applyClipboardSetting starts or stops the clipboard monitor to match the
// current ClipboardMonitor preference.
func (mw *MainWindow) applyClipboardSetting() {
	if mw.settings.ClipboardMonitor && mw.clipStop == nil {
		mw.clipStop = make(chan struct{})
		// Seed with current content so we don't prompt for what's already there.
		mw.clipLast = mw.window.Clipboard().Content()
		go mw.monitorClipboard(mw.clipStop)
	} else if !mw.settings.ClipboardMonitor && mw.clipStop != nil {
		close(mw.clipStop)
		mw.clipStop = nil
	}
}

// monitorClipboard polls the clipboard for new URLs and offers to add them.
func (mw *MainWindow) monitorClipboard(stop chan struct{}) {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			content := strings.TrimSpace(mw.window.Clipboard().Content())
			if content == "" || content == mw.clipLast {
				continue
			}
			mw.clipLast = content
			if !looksLikeURL(content) {
				continue
			}
			url := content
			fyne.Do(func() {
				dialog.ShowConfirm("URL Detected",
					fmt.Sprintf("Add this download?\n\n%s", url),
					func(yes bool) {
						if yes {
							ShowAddDialogWithURL(mw, url)
						}
					}, mw.window)
			})
		}
	}
}

// looksLikeURL reports whether s is a single http(s) URL.
func looksLikeURL(s string) bool {
	if strings.ContainsAny(s, " \n\t") {
		return false
	}
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func (mw *MainWindow) updateStatusBar() {
	var downloading, queued, paused, completed, failed int
	var totalSpeed float64
	for _, row := range mw.downloads {
		switch row.status {
		case rowStatusDownloading:
			downloading++
			totalSpeed += row.speedBps
		case rowStatusQueued:
			queued++
		case rowStatusPaused:
			paused++
		case rowStatusCompleted:
			completed++
		case rowStatusFailed:
			failed++
		}
	}
	if len(mw.downloads) == 0 {
		mw.statusBar.SetText("Ready — add a URL to start downloading")
		return
	}
	msg := fmtf("Total: %d downloading, %d queued, %d paused, %d completed", downloading, queued, paused, completed)
	if failed > 0 {
		msg += fmtf(", %d failed", failed)
	}
	if downloading > 0 && totalSpeed > 0 {
		msg += fmtf("  •  ↓ %s", util.FormatSpeed(totalSpeed))
	}
	mw.statusBar.SetText(msg)
}

// refreshStatusBar is safe to call from any goroutine (must be called inside fyne.Do).
func (mw *MainWindow) refreshStatusBar() {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.updateStatusBar()
}

// restoreHistory loads previous download entries and renders them as static rows.
func (mw *MainWindow) restoreHistory() {
	entries := mw.history.Entries()
	for _, e := range entries {
		row := RestoreDownloadRow(mw, e)
		mw.mu.Lock()
		mw.downloads = append(mw.downloads, row)
		mw.list.Add(row.container)
		mw.mu.Unlock()
	}
	mw.mu.Lock()
	mw.updateStatusBar()
	mw.mu.Unlock()
}

// checkForUpdateSilent runs a background check once per 24 hours.
func (mw *MainWindow) checkForUpdateSilent() {
	prefs := mw.app.Preferences()
	lastCheck := prefs.String(prefLastUpdateCk)
	if lastCheck != "" {
		if t, err := time.Parse(time.RFC3339, lastCheck); err == nil {
			if time.Since(t) < updateCheckEvery {
				return
			}
		}
	}
	info, err := CheckForUpdate(appVersion)
	if err != nil || info == nil {
		return
	}
	prefs.SetString(prefLastUpdateCk, time.Now().Format(time.RFC3339))
	fyne.Do(func() {
		mw.showUpdateBanner(info)
	})
}

// checkForUpdateManual triggers an immediate update check with user feedback.
func (mw *MainWindow) checkForUpdateManual() {
	go func() {
		info, err := CheckForUpdate(appVersion)
		if err != nil {
			fyne.Do(func() {
				dialog.ShowError(fmt.Errorf("update check failed: %w", err), mw.window)
			})
			return
		}
		if info == nil {
			fyne.Do(func() {
				dialog.ShowInformation("Up to Date", fmt.Sprintf("You are running the latest version (%s).", appVersion), mw.window)
			})
			return
		}
		mw.app.Preferences().SetString(prefLastUpdateCk, time.Now().Format(time.RFC3339))
		fyne.Do(func() {
			mw.showUpdateBanner(info)
		})
	}()
}

func (mw *MainWindow) showUpdateBanner(info *ReleaseInfo) {
	label := widget.NewLabel(fmt.Sprintf("Update available: %s", info.TagName))
	label.TextStyle = fyne.TextStyle{Bold: true}

	var link *widget.Hyperlink
	link = widget.NewHyperlink("View Release", nil)
	if parsed, err := url.Parse(info.HTMLURL); err == nil {
		link.URL = parsed
	}

	dismiss := widget.NewButton("✕", func() {
		mw.updateBanner.RemoveAll()
		mw.updateBanner.Refresh()
	})

	mw.updateBanner.RemoveAll()
	mw.updateBanner.Add(label)
	mw.updateBanner.Add(link)
	mw.updateBanner.Add(layout.NewSpacer())
	mw.updateBanner.Add(dismiss)
	mw.updateBanner.Refresh()
}

// ShowDeleteDialog shows a confirmation dialog when removing a download.
func ShowDeleteDialog(mw *MainWindow, row *DownloadRow) {
	// Determine if file exists on disk.
	filePath := row.cfg.OutputPath
	statusText := "completed"
	if row.status == rowStatusFailed || row.status == rowStatusPaused || row.status == rowStatusDownloading {
		statusText = "partial"
	}

	content := widget.NewLabel(fmt.Sprintf("Remove \"%s\" from downloads?", filenameFromPath(filePath)))

	removeEntryBtn := widget.NewButton("Remove entry only", nil)
	deleteFileBtn := widget.NewButton("Delete "+statusText+" file from disk", nil)
	cancelBtn := widget.NewButton("Cancel", nil)

	d := dialog.NewCustomWithoutButtons("Remove Download", container.NewVBox(
		content,
		widget.NewSeparator(),
		removeEntryBtn,
		deleteFileBtn,
		cancelBtn,
	), mw.window)

	removeEntryBtn.OnTapped = func() {
		row.Cancel()
		mw.RemoveDownloadRow(row)
		mw.history.Remove(row.historyID)
		d.Hide()
	}
	deleteFileBtn.OnTapped = func() {
		row.Cancel()
		mw.RemoveDownloadRow(row)
		mw.history.Remove(row.historyID)
		deleteFileAndResume(filePath)
		d.Hide()
	}
	cancelBtn.OnTapped = func() {
		d.Hide()
	}

	d.Resize(fyne.NewSize(400, 200))
	d.Show()
}

// startDownloadWithDuplicateCheck checks if the output file already exists
// (on disk or in history) and prompts the user before redownloading.
func (mw *MainWindow) startDownloadWithDuplicateCheck(cfg engine.DownloadConfig) {
	existing := mw.history.FindByOutputPath(cfg.OutputPath)
	fileOnDisk := false
	if _, err := os.Stat(cfg.OutputPath); err == nil {
		fileOnDisk = true
	}

	if existing != nil || fileOnDisk {
		var msg string
		switch {
		case existing != nil && existing.Status == "completed" && fileOnDisk:
			msg = fmt.Sprintf("File \"%s\" is already downloaded.\nDo you want to redownload it?", filenameFromPath(cfg.OutputPath))
		case existing != nil && existing.Status == "completed" && !fileOnDisk:
			msg = fmt.Sprintf("File \"%s\" was previously downloaded but no longer exists on disk.\nDo you want to redownload it?", filenameFromPath(cfg.OutputPath))
		case fileOnDisk:
			msg = fmt.Sprintf("File \"%s\" already exists on disk.\nDo you want to overwrite it?", filenameFromPath(cfg.OutputPath))
		default:
			msg = fmt.Sprintf("A download for \"%s\" already exists (status: %s).\nDo you want to start a new download?", filenameFromPath(cfg.OutputPath), existing.Status)
		}

		dialog.ShowConfirm("File Already Exists", msg, func(yes bool) {
			if yes {
				// Remove old history entry to avoid duplicates in the list.
				if existing != nil {
					mw.removeRowByHistoryID(existing.ID)
					mw.history.Remove(existing.ID)
				}
				row := NewDownloadRow(mw, cfg)
				mw.AddDownloadRow(row)
			}
		}, mw.window)
		return
	}

	row := NewDownloadRow(mw, cfg)
	mw.AddDownloadRow(row)
}

// removeRowByHistoryID removes the download row matching the history ID from the UI list.
func (mw *MainWindow) removeRowByHistoryID(id string) {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	for i, r := range mw.downloads {
		if r.historyID == id {
			r.Cancel()
			mw.list.Remove(r.container)
			mw.downloads = append(mw.downloads[:i], mw.downloads[i+1:]...)
			break
		}
	}
}

const appVersion = "2.4.0"

func showAboutDialog(mw *MainWindow) {
	var logoImg *canvas.Image
	if data, err := iconFS.ReadFile("assets/icon.png"); err == nil {
		res := fyne.NewStaticResource("icon.png", data)
		logoImg = canvas.NewImageFromResource(res)
		logoImg.SetMinSize(fyne.NewSize(128, 128))
		logoImg.FillMode = canvas.ImageFillContain
	}

	name := widget.NewLabelWithStyle("Swiftload Downloader", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	ver := widget.NewLabelWithStyle(fmt.Sprintf("Version %s", appVersion), fyne.TextAlignCenter, fyne.TextStyle{})
	dev := widget.NewLabelWithStyle("Developer: Bhayanak", fyne.TextAlignCenter, fyne.TextStyle{})

	link := widget.NewHyperlink("github.com/bhayanak/swiftload-downloader", nil)
	if parsed, err := url.Parse("https://github.com/bhayanak/swiftload-downloader"); err == nil {
		link.URL = parsed
	}
	link.Alignment = fyne.TextAlignCenter

	license := widget.NewLabelWithStyle("License: MIT", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	items := []fyne.CanvasObject{
		widget.NewSeparator(),
		name,
		ver,
		widget.NewSeparator(),
		dev,
		link,
		widget.NewSeparator(),
		license,
	}
	if logoImg != nil {
		items = append([]fyne.CanvasObject{logoImg}, items...)
	}

	content := container.NewVBox(items...)
	d := dialog.NewCustom("About Swiftload", "Close", content, mw.window)
	d.Resize(fyne.NewSize(350, 400))
	d.Show()
}

// deleteFileAndResume removes the output file and its .gdown.json resume sidecar.
func deleteFileAndResume(outputPath string) {
	_ = os.Remove(outputPath)
	_ = os.Remove(outputPath + ".gdown.json")
}
