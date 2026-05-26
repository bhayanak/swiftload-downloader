package gui

import (
	"fmt"
	"net/url"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
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
		widget.NewButton("+ Add URL", func() {
			ShowAddDialog(mw)
		}),
		widget.NewButton("⏸ Pause All", func() {
			mw.pauseAll()
		}),
		widget.NewButton("▶ Resume All", func() {
			mw.resumeAll()
		}),
		widget.NewButton("⚙ Settings", func() {
			ShowSettingsDialog(mw)
		}),
		layout.NewSpacer(),
		widget.NewButton("🔄 Check Update", func() {
			mw.checkForUpdateManual()
		}),
		widget.NewButton("ℹ About", func() {
			showAboutDialog(mw)
		}),
	)

	// Header row.
	header := container.NewGridWithColumns(6,
		widget.NewLabelWithStyle("Filename", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Size", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Progress", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Speed", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("ETA", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Actions", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
	)

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

	// Background update check (once per 24h).
	go mw.checkForUpdateSilent()

	return mw
}

// Show displays the main window.
func (mw *MainWindow) Show() {
	mw.window.Show()
}

// AddDownloadRow adds a new download row to the list.
func (mw *MainWindow) AddDownloadRow(row *DownloadRow) {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	mw.downloads = append(mw.downloads, row)
	mw.list.Add(row.container)
	mw.updateStatusBar()
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
	defer mw.mu.Unlock()
	for _, row := range mw.downloads {
		row.Pause()
	}
}

func (mw *MainWindow) resumeAll() {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	for _, row := range mw.downloads {
		row.Resume()
	}
}

func (mw *MainWindow) cancelAll() {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	for _, row := range mw.downloads {
		row.Cancel()
	}
}

func (mw *MainWindow) updateStatusBar() {
	var downloading, paused, completed, failed int
	for _, row := range mw.downloads {
		switch row.status {
		case rowStatusDownloading:
			downloading++
		case rowStatusPaused:
			paused++
		case rowStatusCompleted:
			completed++
		case rowStatusFailed:
			failed++
		}
	}
	msg := fmtf("Total: %d downloading, %d paused, %d completed", downloading, paused, completed)
	if failed > 0 {
		msg += fmtf(", %d failed", failed)
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

const appVersion = "2.0.0"

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
