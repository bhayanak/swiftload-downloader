package gui

import (
	"embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

//go:embed assets/icon.png
var iconFS embed.FS

const appID = "com.swiftload.downloader"

// App wraps the fyne application and main window.
type App struct {
	fyneApp    fyne.App
	mainWindow *MainWindow
}

// NewApp creates and configures the Swiftload Downloader GUI application.
func NewApp() *App {
	a := app.NewWithID(appID)

	// Set app icon from embedded asset.
	if data, err := iconFS.ReadFile("assets/icon.png"); err == nil {
		a.SetIcon(fyne.NewStaticResource("icon.png", data))
	}

	// Load saved theme preference.
	settings := LoadSettings(a)
	ApplyTheme(a, settings.Theme)

	guiApp := &App{
		fyneApp: a,
	}
	guiApp.mainWindow = NewMainWindow(a)
	return guiApp
}

// Run starts the GUI event loop. Blocks until the window is closed.
func (a *App) Run() {
	a.mainWindow.Show()
	a.fyneApp.Run()
}
