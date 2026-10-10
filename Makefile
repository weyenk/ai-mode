# Build the ai-mode Go binary into dist/.
.PHONY: build test eval install clean plan-example

build:
	go build -trimpath -ldflags "-s -w" -o dist/ai-mode ./cmd/ai-mode

test:
	go vet ./... && go test ./...

# Lint the drafting-plans worked example and verify its code tasks against this repo (runs the repo's tests on a copy).
plan-example: build
	dist/ai-mode plan lint skills/drafting-plans/references/example-plan.md
	dist/ai-mode plan verify skills/drafting-plans/references/example-plan.md --task all --repo .

# Needs the active profile (ai-mode use dev-shop). Tune against classify.jsonl; check holdout afterwards.
eval: build
	dist/ai-mode classify --eval evals/classify.jsonl
	@echo
	dist/ai-mode classify --eval evals/classify-holdout.jsonl

# Symlink so the binary can find presets/ next to the repo.
install: build
	mkdir -p $(HOME)/.local/bin
	ln -sfn $(CURDIR)/dist/ai-mode $(HOME)/.local/bin/ai-mode

clean:
	rm -rf dist
