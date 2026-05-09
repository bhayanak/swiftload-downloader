.PHONY: build build-cli build-gui test clean install cross-cli

VERSION ?= 2.0.0
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

# ── Build ─────────────────────────────────────────────────────────────────────

build: build-cli build-gui

build-cli:
	go build $(LDFLAGS) -o bin/gdown ./cmd/gdown

build-gui:
	go build $(LDFLAGS) -o bin/gdown-gui ./cmd/gdown-gui

# ── Install CLI globally ──────────────────────────────────────────────────────

install:
	go install $(LDFLAGS) ./cmd/gdown

# ── Test ──────────────────────────────────────────────────────────────────────

test:
	go test ./pkg/... ./internal/... -v -count=1

# ── Lint ──────────────────────────────────────────────────────────────────────

lint:
	go vet ./...

# ── Cross-compile CLI (pure Go, no CGO needed) ───────────────────────────────

cross-cli:
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o dist/gdown-darwin-arm64    ./cmd/gdown
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o dist/gdown-darwin-amd64    ./cmd/gdown
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o dist/gdown-linux-amd64     ./cmd/gdown
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o dist/gdown-linux-arm64     ./cmd/gdown
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/gdown-windows-amd64.exe ./cmd/gdown

# ── Cross-compile GUI (requires fyne-cross + Docker) ─────────────────────────

cross-gui:
	fyne-cross darwin  -arch arm64,amd64 ./cmd/gdown-gui
	fyne-cross linux   -arch amd64       ./cmd/gdown-gui
	fyne-cross windows -arch amd64       ./cmd/gdown-gui

# ── Package macOS GUI as .app bundle ─────────────────────────────────────────

package-macos:
	fyne package -os darwin -name "Swiftload" -appID com.swiftload.downloader -sourceDir ./cmd/gdown-gui

# ── Clean ─────────────────────────────────────────────────────────────────────

clean:
	rm -rf bin/ dist/ fyne-cross/
	go clean
