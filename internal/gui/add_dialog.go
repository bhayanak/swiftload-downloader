package gui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/yadava/gdown/pkg/engine"
)

// ShowAddDialog shows a dialog to add a new download URL.
func ShowAddDialog(mw *MainWindow) {
	urlEntry := widget.NewEntry()
	urlEntry.SetPlaceHolder("https://example.com/file.iso")

	outputEntry := widget.NewEntry()
	outputEntry.SetPlaceHolder("file.iso (auto-detected from URL if empty)")

	parallelCheck := widget.NewCheck("Parallel download", nil)
	parallelCheck.SetChecked(true)

	workersEntry := widget.NewEntry()
	workersEntry.SetText("16")

	// Use a variable to hold dialog reference so OnSubmit can close it.
	var d dialog.Dialog

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "URL", Widget: urlEntry},
			{Text: "Save as", Widget: outputEntry},
			{Text: "Mode", Widget: parallelCheck},
			{Text: "Workers", Widget: workersEntry},
		},
		OnSubmit: func() {
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
				URL:        url,
				OutputPath: output,
				Workers:    workers,
				Parallel:   parallelCheck.Checked,
			}

			row := NewDownloadRow(mw, cfg)
			mw.AddDownloadRow(row)

			// Close the dialog after adding.
			if d != nil {
				d.Hide()
			}
		},
	}

	d = dialog.NewCustom("Add Download", "Cancel", container.NewVBox(form), mw.window)
	d.Resize(fyne.NewSize(600, 300))
	d.Show()
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
