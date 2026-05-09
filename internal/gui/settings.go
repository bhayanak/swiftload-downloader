package gui

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Preference keys.
const (
	prefDownloadDir    = "download_dir"
	prefMaxConcurrent  = "max_concurrent"
	prefDefaultWorkers = "default_workers"
	prefTheme          = "theme"
)

// AppSettings holds the in-memory copy of user preferences.
type AppSettings struct {
	DownloadDir    string
	MaxConcurrent  int
	DefaultWorkers int
	Theme          string
}

// LoadSettings reads settings from Fyne app preferences.
func LoadSettings(a fyne.App) AppSettings {
	prefs := a.Preferences()
	return AppSettings{
		DownloadDir:    prefs.StringWithFallback(prefDownloadDir, "./"),
		MaxConcurrent:  prefs.IntWithFallback(prefMaxConcurrent, 3),
		DefaultWorkers: prefs.IntWithFallback(prefDefaultWorkers, 16),
		Theme:          prefs.StringWithFallback(prefTheme, "System"),
	}
}

// SaveSettings persists settings to Fyne app preferences.
func SaveSettings(a fyne.App, s AppSettings) {
	prefs := a.Preferences()
	prefs.SetString(prefDownloadDir, s.DownloadDir)
	prefs.SetInt(prefMaxConcurrent, s.MaxConcurrent)
	prefs.SetInt(prefDefaultWorkers, s.DefaultWorkers)
	prefs.SetString(prefTheme, s.Theme)
}

// ApplyTheme sets the Fyne theme based on the theme name.
func ApplyTheme(a fyne.App, themeName string) {
	switch themeName {
	case "Light":
		a.Settings().SetTheme(theme.LightTheme())
	case "Dark":
		a.Settings().SetTheme(theme.DarkTheme())
	default:
		a.Settings().SetTheme(theme.DefaultTheme())
	}
}

// ShowSettingsDialog displays the settings/preferences dialog.
func ShowSettingsDialog(mw *MainWindow) {
	s := mw.settings

	downloadDirEntry := widget.NewEntry()
	downloadDirEntry.SetText(s.DownloadDir)
	downloadDirEntry.SetPlaceHolder("Default download directory")

	maxConcurrentEntry := widget.NewEntry()
	maxConcurrentEntry.SetText(strconv.Itoa(s.MaxConcurrent))

	defaultWorkersEntry := widget.NewEntry()
	defaultWorkersEntry.SetText(strconv.Itoa(s.DefaultWorkers))

	themeSelect := widget.NewSelect([]string{"System", "Light", "Dark"}, nil)
	themeSelect.SetSelected(s.Theme)

	var d dialog.Dialog

	saveBtn := widget.NewButton("Save", func() {
		newSettings := AppSettings{
			DownloadDir:    downloadDirEntry.Text,
			MaxConcurrent:  parseIntFallback(maxConcurrentEntry.Text, 3),
			DefaultWorkers: parseIntFallback(defaultWorkersEntry.Text, 16),
			Theme:          themeSelect.Selected,
		}
		SaveSettings(mw.app, newSettings)
		mw.settings = newSettings
		ApplyTheme(mw.app, newSettings.Theme)
		if d != nil {
			d.Hide()
		}
	})
	saveBtn.Importance = widget.HighImportance

	form := container.NewVBox(
		widget.NewLabelWithStyle("Settings", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),

		widget.NewLabel("Default download directory:"),
		downloadDirEntry,

		widget.NewLabel("Max concurrent downloads:"),
		maxConcurrentEntry,

		widget.NewLabel("Default workers per download:"),
		defaultWorkersEntry,

		widget.NewLabel("Theme:"),
		themeSelect,

		widget.NewSeparator(),
		saveBtn,
	)

	d = dialog.NewCustom("Swiftload Settings", "Cancel", form, mw.window)
	d.Resize(fyne.NewSize(500, 450))
	d.Show()
}

func parseIntFallback(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
