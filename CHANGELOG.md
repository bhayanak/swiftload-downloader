# Changelog

All notable changes to Swiftload Downloader will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.4.0] - 2026-08-04

### Added
- Download queue with a configurable max-concurrent limit; extra downloads wait in a "Queued" state and start automatically as slots free up
- Clipboard URL auto-detect (opt-in): watches the clipboard and offers to add copied http(s) links
- Desktop notifications when a download completes or fails (toggle in Settings)
- Retry surfacing: the current retry count is shown inline next to the speed (e.g. `↻2`)
- Per-download real-time speed sparkline graph in each row
- Coloured status badges per row (blue=downloading, green=done, amber=paused, red=failed, slate=queued)
- Card-style rows with hover highlight for a more modern list
- Multi-mirror downloads: supply additional mirror URLs (GUI Mirrors field / `--mirror` CLI flag) and chunks are spread across hosts
- Engine resume option `WithMirrors`

### Changed
- Add and Settings dialogs now keep their action buttons fixed while only the fields scroll, for smoother scrolling
- Status bar now also reports the queued count

### Fixed
- Column alignment between the header and download rows after the card layout change

## [2.3.0] - 2026-08-04

### Added
- HTTP Basic Authentication: `--user`/`--password` CLI flags and Username/Password fields in the Add dialog (for password-protected sites)
- Custom request headers: `--header`/`-H` CLI flag (repeatable) and a multi-line Headers field in the Add dialog
- Speed limiting: token-bucket throttle via `--limit` CLI flag (e.g. `500k`, `2m`) and a Speed limit field in the Add dialog
- Resume options for the engine API: `WithAuth`, `WithHeaders`, `WithSpeedLimit`
- Modern UI: brand accent theme (light/dark), icon toolbar and per-row action buttons, aggregate download speed in the status bar, and an empty-state hint

### Fixed
- **Critical resume bug**: paused parallel downloads no longer restart from zero — mid-chunk progress is now persisted to the resume sidecar and reused on resume
- GUI Resume reuses the in-memory config (including credentials) so authenticated downloads continue correctly after pause

### Security
- Credentials and custom headers are never written to disk (history or resume state); they are re-supplied via CLI flags or the dialog when resuming

## [2.2.0] - 2026-05-26

### Added
- Duplicate download detection: warns when adding a URL that targets an existing file
- File-deleted detection: clicking "Open Folder" on a missing file offers to redownload
- Download history persistence across app restarts
- Delete dialog with options: remove entry only, delete file from disk, or cancel
- Check for update feature (silent 24h background check + manual toolbar button)
- Update banner in main window with link to latest release
- Size and elapsed time shown for completed downloads
- Elapsed time shown during downloads when ETA is unknown (e.g. no Content-Length)

### Fixed
- Resume now auto-restarts fresh when no resume state exists (e.g. serial downloads from GitHub)
- Error messages in ETA column truncated to prevent window expansion
- UI column alignment: Size, Speed, ETA labels centered; filename truncated with ellipsis
- Size column shows downloaded bytes when server doesn't provide Content-Length

## [2.1.0] - 2026-05-25

### Added
- Persistent download history (history.json in app storage)
- Auto-update check via GitHub Releases API
- About dialog with version info and links

### Fixed
- GoReleaser 403 error (corrected repo name to swiftload-downloader)
- Parallel download redirect fix (use final URL after redirects for chunks)
- Module renamed from gdown to swiftload-downloader

## [2.0.0] - 2026-05-24

### Added
- GUI application (Fyne v2.7.3) with download list, progress bars, speed/ETA display
- Parallel chunked downloads with configurable worker count
- Pause/Resume/Cancel/Restart per download
- Serial fallback when server doesn't support range requests
- Resume state saved as `.gdown.json` sidecar files
- Checksum verification (MD5/SHA-256)
- Proxy support (environment or manual configuration)
- Settings dialog (download directory, workers, buffer size, proxy, checksum algo)
- CLI tool with Cobra (download, resume, version subcommands)
- Cross-platform builds (macOS, Linux, Windows)
- GitHub Actions CI/CD with GoReleaser
- Unit tests with 84%+ coverage on core packages

## [1.0.0] - 2026-05-20

### Added
- Initial release
- Basic single-stream HTTP download
- CLI interface
