# clai Makefile

BINARY := clai
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/maxrodrigo/clai/internal/version.Version=$(VERSION)

PREFIX := $(HOME)/.local
BINDIR := $(PREFIX)/bin
DATADIR := $(PREFIX)/share/clai

.DEFAULT_GOAL := help

build: ## Build clai binary
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/clai

run: ## Run without building (use ARGS="...")
	@CLAI_DATA_DIR=$(CURDIR)/share/clai go run ./cmd/clai $(ARGS)

test: ## Run tests
	go test -race ./...

lint: ## Run linter
	golangci-lint run

check: lint test ## Lint + test

ci: lint test tidy-check build-check ## Run full CI locally

tidy: ## Tidy go.mod
	go mod tidy

tidy-check: ## Verify go.mod is tidy
	@go mod tidy
	@git diff --exit-code go.mod go.sum || (echo "error: go.mod/go.sum not tidy - run 'make tidy' and commit" >&2 && exit 1)

build-check: ## Verify all packages compile
	@go build -o /dev/null ./...

install: build ## Install binary and data to PREFIX (default ~/.local)
	install -d $(BINDIR) $(DATADIR)
	install -m 755 $(BINARY) $(BINDIR)/$(BINARY)
	cp -r share/clai/* $(DATADIR)/

uninstall: ## Remove installed files
	rm -f $(BINDIR)/$(BINARY)
	rm -rf $(DATADIR)

clean: ## Remove built binary
	rm -f $(BINARY)

release: ## Preview and tag a release
	@git diff --exit-code --quiet || (echo "error: working tree is dirty" >&2 && exit 1)
	@git cliff --bump --unreleased
	@VERSION=v$$(git cliff --bumped-version) && \
	printf "\nVersion [$$VERSION]: " && read -r ans && \
	VERSION=$${ans:-$$VERSION} && \
	git tag -m "$$VERSION" "$$VERSION" && git push origin "$$VERSION" && \
	echo "\nTagged $$VERSION. Draft release will appear on GitHub."

help: ## Show commands
	@awk 'BEGIN {FS = ":.*##"} /^[a-z][a-z-]+:.*##/ {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build run test lint check ci tidy tidy-check build-check install uninstall clean release help
