package gui

import (
	"net/http"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/bhayanak/swiftload-downloader/pkg/engine"
)

// ShowAddDialog shows a dialog to add a new download URL.
func ShowAddDialog(mw *MainWindow) {
	ShowAddDialogWithURL(mw, "")
}

// ShowAddDialogWithURL shows the add dialog with the URL field pre-filled.
func ShowAddDialogWithURL(mw *MainWindow, prefillURL string) {
	urlEntry := widget.NewEntry()
	urlEntry.SetPlaceHolder("https://example.com/file.iso")
	if prefillURL != "" {
		urlEntry.SetText(prefillURL)
	}

	outputEntry := widget.NewEntry()
	outputEntry.SetPlaceHolder("file.iso (auto-detected from URL if empty)")

	parallelCheck := widget.NewCheck("Parallel download", nil)
	parallelCheck.SetChecked(true)

	workersEntry := widget.NewEntry()
	workersEntry.SetText("16")

	checksumEntry := widget.NewEntry()
	checksumEntry.SetPlaceHolder("Optional: paste expected hash for verification (change algorithm from settings)")

	// Authentication (HTTP basic-auth).
	userEntry := widget.NewEntry()
	userEntry.SetPlaceHolder("Username (optional)")
	passEntry := widget.NewPasswordEntry()
	passEntry.SetPlaceHolder("Password (optional)")

	// Custom headers, one "Key: Value" per line.
	headersEntry := widget.NewMultiLineEntry()
	headersEntry.SetPlaceHolder("Optional custom headers, one per line:\nCookie: session=abc\nReferer: https://example.com")
	headersEntry.SetMinRowsVisible(2)

	// Mirror URLs, one per line (same file served from multiple hosts).
	mirrorsEntry := widget.NewMultiLineEntry()
	mirrorsEntry.SetPlaceHolder("Optional mirror URLs, one per line (same file):\nhttps://mirror1.example.com/file.iso")
	mirrorsEntry.SetMinRowsVisible(2)

	// Speed limit, e.g. 500k / 2m (empty = unlimited).
	limitEntry := widget.NewEntry()
	limitEntry.SetPlaceHolder("e.g. 500k, 2m (empty = unlimited)")

	form := widget.NewForm(
		widget.NewFormItem("URL", urlEntry),
		widget.NewFormItem("Save as", outputEntry),
		widget.NewFormItem("Mode", parallelCheck),
		widget.NewFormItem("Workers", workersEntry),
		widget.NewFormItem("Checksum", checksumEntry),
		widget.NewFormItem("Username", userEntry),
		widget.NewFormItem("Password", passEntry),
		widget.NewFormItem("Headers", headersEntry),
		widget.NewFormItem("Mirrors", mirrorsEntry),
		widget.NewFormItem("Speed limit", limitEntry),
	)

	scrollable := container.NewVScroll(form)
	scrollable.SetMinSize(fyne.NewSize(560, 360))

	d := dialog.NewCustomConfirm("Add Download", "Add", "Cancel", scrollable, func(add bool) {
		if !add {
			return
		}
		url := strings.TrimSpace(urlEntry.Text)
		if url == "" {
			return
		}

		output := strings.TrimSpace(outputEntry.Text)
		if output == "" {
			output = guessFilename(url)
		}

		// Prepend download directory if set.
		if mw.settings.DownloadDir != "" && mw.settings.DownloadDir != "./" {
			output = strings.TrimRight(mw.settings.DownloadDir, "/") + "/" + output
		}

		workers := mw.settings.DefaultWorkers
		if w := strings.TrimSpace(workersEntry.Text); w != "" {
			if n := parseInt(w); n > 0 {
				workers = n
			}
		}

		cfg := engine.DownloadConfig{
			URL:          url,
			OutputPath:   output,
			Workers:      workers,
			Parallel:     parallelCheck.Checked,
			BufSize:      int64(mw.settings.BufSizeMB) * 1024 * 1024,
			UseProxy:     mw.settings.ProxyMode == "environment",
			ProxyURL:     "",
			Checksum:     strings.TrimSpace(checksumEntry.Text),
			ChecksumAlgo: mw.settings.ChecksumAlgo,
			Username:     strings.TrimSpace(userEntry.Text),
			Password:     passEntry.Text,
			Headers:      parseHeaderLines(headersEntry.Text),
			Mirrors:      parseMirrorLines(mirrorsEntry.Text),
			SpeedLimit:   parseSpeedLimit(limitEntry.Text),
		}
		if mw.settings.ProxyMode == "manual" && mw.settings.ProxyURL != "" {
			cfg.UseProxy = true
			cfg.ProxyURL = mw.settings.ProxyURL
		}

		// Check if this file already exists on disk or in history.
		mw.startDownloadWithDuplicateCheck(cfg)
	}, mw.window)

	d.Resize(fyne.NewSize(620, 480))
	d.Show()
}

// parseMirrorLines splits newline-separated URLs into a slice, trimming blanks.
func parseMirrorLines(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// parseHeaderLines parses newline-separated "Key: Value" pairs into an http.Header.
func parseHeaderLines(text string) http.Header {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	h := make(http.Header)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if key != "" {
			h.Add(key, val)
		}
	}
	if len(h) == 0 {
		return nil
	}
	return h
}

// parseSpeedLimit parses "500k", "2m", "1g" into bytes/sec. Empty/0 => 0.
func parseSpeedLimit(s string) int64 {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "0" {
		return 0
	}
	mult := int64(1)
	switch s[len(s)-1] {
	case 'k':
		mult = 1024
		s = s[:len(s)-1]
	case 'm':
		mult = 1024 * 1024
		s = s[:len(s)-1]
	case 'g':
		mult = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}
	n := parseInt(strings.TrimSpace(s))
	return int64(n) * mult
}

// guessFilename extracts a filename from the URL.
func guessFilename(rawURL string) string {
	u := rawURL
	if idx := strings.IndexAny(u, "?#"); idx != -1 {
		u = u[:idx]
	}
	if idx := strings.LastIndex(u, "/"); idx != -1 {
		u = u[idx+1:]
	}
	if u == "" {
		u = "download"
	}
	return u
}

// parseInt is a simple string-to-int parser.
func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
