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
	prefBufSizeMB      = "bufsize_mb"
	prefProxyMode      = "proxy_mode"
	prefProxyURL       = "proxy_url"
	prefChecksumAlgo   = "checksum_algo"
)

// AppSettings holds the in-memory copy of user preferences.
type AppSettings struct {
	DownloadDir    string
	MaxConcurrent  int
	DefaultWorkers int
	Theme          string
	BufSizeMB      int
	ProxyMode      string // "none", "environment", "manual"
	ProxyURL       string // used when ProxyMode == "manual"
	ChecksumAlgo   string
}

// LoadSettings reads settings from Fyne app preferences.
func LoadSettings(a fyne.App) AppSettings {
	prefs := a.Preferences()
	return AppSettings{
		DownloadDir:    prefs.StringWithFallback(prefDownloadDir, "./"),
		MaxConcurrent:  prefs.IntWithFallback(prefMaxConcurrent, 3),
		DefaultWorkers: prefs.IntWithFallback(prefDefaultWorkers, 16),
		Theme:          prefs.StringWithFallback(prefTheme, "System"),
		BufSizeMB:      prefs.IntWithFallback(prefBufSizeMB, 4),
		ProxyMode:      prefs.StringWithFallback(prefProxyMode, "none"),
		ProxyURL:       prefs.StringWithFallback(prefProxyURL, ""),
		ChecksumAlgo:   prefs.StringWithFallback(prefChecksumAlgo, "sha256"),
	}
}

// SaveSettings persists settings to Fyne app preferences.
func SaveSettings(a fyne.App, s AppSettings) {
	prefs := a.Preferences()
	prefs.SetString(prefDownloadDir, s.DownloadDir)
	prefs.SetInt(prefMaxConcurrent, s.MaxConcurrent)
	prefs.SetInt(prefDefaultWorkers, s.DefaultWorkers)
	prefs.SetString(prefTheme, s.Theme)
	prefs.SetInt(prefBufSizeMB, s.BufSizeMB)
	prefs.SetString(prefProxyMode, s.ProxyMode)
	prefs.SetString(prefProxyURL, s.ProxyURL)
	prefs.SetString(prefChecksumAlgo, s.ChecksumAlgo)
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
	downloadDirEntry.SetPlaceHolder("/Users/you/Downloads")

	maxConcurrentEntry := widget.NewEntry()
	maxConcurrentEntry.SetText(strconv.Itoa(s.MaxConcurrent))

	defaultWorkersEntry := widget.NewEntry()
	defaultWorkersEntry.SetText(strconv.Itoa(s.DefaultWorkers))

	themeSelect := widget.NewSelect([]string{"System", "Light", "Dark"}, nil)
	themeSelect.SetSelected(s.Theme)

	bufSizeEntry := widget.NewEntry()
	bufSizeEntry.SetText(strconv.Itoa(s.BufSizeMB))

	checksumAlgoSelect := widget.NewSelect([]string{"sha256", "md5"}, nil)
	checksumAlgoSelect.SetSelected(s.ChecksumAlgo)

	// Proxy settings.
	proxyURLEntry := widget.NewEntry()
	proxyURLEntry.SetPlaceHolder("http://proxy.example.com:8080")
	proxyURLEntry.SetText(s.ProxyURL)

	proxyModeSelect := widget.NewSelect([]string{"none", "environment", "manual"}, func(val string) {
		if val == "manual" {
			proxyURLEntry.Enable()
		} else {
			proxyURLEntry.Disable()
		}
	})
	proxyModeSelect.SetSelected(s.ProxyMode)
	if s.ProxyMode != "manual" {
		proxyURLEntry.Disable()
	}

	var d dialog.Dialog

	saveBtn := widget.NewButton("Save", func() {
		newSettings := AppSettings{
			DownloadDir:    downloadDirEntry.Text,
			MaxConcurrent:  parseIntFallback(maxConcurrentEntry.Text, 3),
			DefaultWorkers: parseIntFallback(defaultWorkersEntry.Text, 16),
			Theme:          themeSelect.Selected,
			BufSizeMB:      parseIntFallback(bufSizeEntry.Text, 4),
			ProxyMode:      proxyModeSelect.Selected,
			ProxyURL:       proxyURLEntry.Text,
			ChecksumAlgo:   checksumAlgoSelect.Selected,
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

		widget.NewLabel("Read buffer size (MB):"),
		bufSizeEntry,

		widget.NewLabel("Checksum algorithm:"),
		checksumAlgoSelect,

		widget.NewSeparator(),
		widget.NewLabelWithStyle("Proxy", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Proxy mode:"),
		proxyModeSelect,
		widget.NewLabel("Manual proxy URL:"),
		proxyURLEntry,

		widget.NewSeparator(),
		widget.NewLabel("Theme:"),
		themeSelect,

		widget.NewSeparator(),
		saveBtn,
	)

	scrollable := container.NewVScroll(form)
	scrollable.SetMinSize(fyne.NewSize(460, 500))

	d = dialog.NewCustom("Swiftload Settings", "Cancel", scrollable, mw.window)
	d.Resize(fyne.NewSize(520, 560))
	d.Show()
}

func parseIntFallback(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
