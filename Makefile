# Build the ai-mode Go binary. Output goes to dist/ (bin/ai-mode is still the Python CLI).
.PHONY: build test install clean

build:
	go build -trimpath -ldflags "-s -w" -o dist/ai-mode ./cmd/ai-mode

test:
	go vet ./... && go test ./...

# Symlink so the binary can find presets/ next to the repo; replaces ~/.local/bin/ai-mode.
install: build
	mkdir -p $(HOME)/.local/bin
	ln -sfn $(CURDIR)/dist/ai-mode $(HOME)/.local/bin/ai-mode

clean:
	rm -rf dist
