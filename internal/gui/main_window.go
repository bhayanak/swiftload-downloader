package gui

import (
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// MainWindow is the primary application window with the download list.
type MainWindow struct {
	window    fyne.Window
	app       fyne.App
	downloads []*DownloadRow
	list      *fyne.Container
	mu        sync.Mutex
	statusBar *widget.Label
	settings  AppSettings
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
	}

	mw.list = container.NewVBox()

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
		container.NewVBox(toolbar, header), // top
		mw.statusBar,                       // bottom
		nil, nil,                           // left, right
		scrollable,                         // center
	)

	w.SetContent(content)
	w.SetCloseIntercept(func() {
		mw.cancelAll()
		w.Close()
	})

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
	var downloading, paused, completed int
	for _, row := range mw.downloads {
		switch row.status {
		case rowStatusDownloading:
			downloading++
		case rowStatusPaused:
			paused++
		case rowStatusCompleted:
			completed++
		}
	}
	mw.statusBar.SetText(
		fmtf("Total: %d downloading, %d paused, %d completed", downloading, paused, completed),
	)
}
