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
	prefNotifications  = "notifications"
	prefClipboard      = "clipboard_monitor"
)

// AppSettings holds the in-memory copy of user preferences.
type AppSettings struct {
	DownloadDir      string
	MaxConcurrent    int
	DefaultWorkers   int
	Theme            string
	BufSizeMB        int
	ProxyMode        string // "none", "environment", "manual"
	ProxyURL         string // used when ProxyMode == "manual"
	ChecksumAlgo     string
	Notifications    bool // desktop notification on complete/fail
	ClipboardMonitor bool // watch clipboard for URLs
}

// LoadSettings reads settings from Fyne app preferences.
func LoadSettings(a fyne.App) AppSettings {
	prefs := a.Preferences()
	return AppSettings{
		DownloadDir:      prefs.StringWithFallback(prefDownloadDir, "./"),
		MaxConcurrent:    prefs.IntWithFallback(prefMaxConcurrent, 3),
		DefaultWorkers:   prefs.IntWithFallback(prefDefaultWorkers, 16),
		Theme:            prefs.StringWithFallback(prefTheme, "System"),
		BufSizeMB:        prefs.IntWithFallback(prefBufSizeMB, 4),
		ProxyMode:        prefs.StringWithFallback(prefProxyMode, "none"),
		ProxyURL:         prefs.StringWithFallback(prefProxyURL, ""),
		ChecksumAlgo:     prefs.StringWithFallback(prefChecksumAlgo, "sha256"),
		Notifications:    prefs.BoolWithFallback(prefNotifications, true),
		ClipboardMonitor: prefs.BoolWithFallback(prefClipboard, false),
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
	prefs.SetBool(prefNotifications, s.Notifications)
	prefs.SetBool(prefClipboard, s.ClipboardMonitor)
}

// ApplyTheme sets the Fyne theme based on the theme name.
func ApplyTheme(a fyne.App, themeName string) {
	switch themeName {
	case "Light":
		a.Settings().SetTheme(newSwiftTheme(theme.VariantLight))
	case "Dark":
		a.Settings().SetTheme(newSwiftTheme(theme.VariantDark))
	default:
		// Follow the OS setting but keep the brand accent.
		variant := theme.VariantDark
		if a.Settings().ThemeVariant() == theme.VariantLight {
			variant = theme.VariantLight
		}
		a.Settings().SetTheme(newSwiftTheme(variant))
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

	notifyCheck := widget.NewCheck("Show desktop notification when a download finishes", nil)
	notifyCheck.SetChecked(s.Notifications)

	clipboardCheck := widget.NewCheck("Watch clipboard and offer to add copied URLs", nil)
	clipboardCheck.SetChecked(s.ClipboardMonitor)

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

	form := container.NewVBox(
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
		widget.NewLabelWithStyle("Behaviour", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		notifyCheck,
		clipboardCheck,

		widget.NewSeparator(),
		widget.NewLabelWithStyle("Proxy", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("Proxy mode:"),
		proxyModeSelect,
		widget.NewLabel("Manual proxy URL:"),
		proxyURLEntry,

		widget.NewSeparator(),
		widget.NewLabel("Theme:"),
		themeSelect,
	)

	scrollable := container.NewVScroll(form)
	scrollable.SetMinSize(fyne.NewSize(460, 420))

	d := dialog.NewCustomConfirm("Swiftload Settings", "Save", "Cancel", scrollable, func(save bool) {
		if !save {
			return
		}
		newSettings := AppSettings{
			DownloadDir:      downloadDirEntry.Text,
			MaxConcurrent:    parseIntFallback(maxConcurrentEntry.Text, 3),
			DefaultWorkers:   parseIntFallback(defaultWorkersEntry.Text, 16),
			Theme:            themeSelect.Selected,
			BufSizeMB:        parseIntFallback(bufSizeEntry.Text, 4),
			ProxyMode:        proxyModeSelect.Selected,
			ProxyURL:         proxyURLEntry.Text,
			ChecksumAlgo:     checksumAlgoSelect.Selected,
			Notifications:    notifyCheck.Checked,
			ClipboardMonitor: clipboardCheck.Checked,
		}
		SaveSettings(mw.app, newSettings)
		mw.settings = newSettings
		ApplyTheme(mw.app, newSettings.Theme)
		mw.applyClipboardSetting()
	}, mw.window)
	d.Resize(fyne.NewSize(540, 560))
	d.Show()
}

func parseIntFallback(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
