# Changelog

All notable changes to Swiftload Downloader will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
