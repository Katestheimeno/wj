# wj — the CLI is the pure-bash `wj` script (nothing to build).
# These targets build and install the optional Go TUI front-end, wj-tui.

PREFIX  ?= $(HOME)/.local
BINDIR  ?= $(PREFIX)/bin
GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

BATS          ?= bats
GOLANGCI_LINT ?= golangci-lint

.PHONY: all tui test test-cli test-go lint cover vet fmt install install-cli install-ui uninstall clean

all: tui

## tui: build the wj-tui binary into ./tui/wj-tui
tui:
	cd tui && $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o wj-tui .

## test: run both the bash (bats) and Go test suites
test: test-cli test-go

## test-cli: run the bash CLI test suite (needs bats-core: https://bats-core.readthedocs.io)
test-cli:
	$(BATS) tests/

## test-go: run the Go test suite (incl. integration tests against ./wj)
test-go:
	cd tui && $(GO) test -race ./...

## cover: Go test suite with a coverage summary
cover:
	cd tui && $(GO) test -coverprofile=coverage.out -covermode=atomic ./... && $(GO) tool cover -func=coverage.out | tail -1

## lint: run golangci-lint over the Go code (needs golangci-lint on PATH)
lint:
	cd tui && $(GOLANGCI_LINT) run ./...

vet:
	cd tui && $(GO) vet ./...

fmt:
	cd tui && $(GO) fmt ./...

## install: install both the bash CLI and the TUI
install: install-cli install-ui

install-cli:
	install -Dm755 wj $(DESTDIR)$(BINDIR)/wj
	install -Dm644 wj.1 $(DESTDIR)$(PREFIX)/share/man/man1/wj.1
	install -Dm644 wj.cfg.example $(DESTDIR)$(PREFIX)/share/doc/wj/wj.cfg.example

install-ui: tui
	install -Dm755 tui/wj-tui $(DESTDIR)$(BINDIR)/wj-tui

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/wj $(DESTDIR)$(BINDIR)/wj-tui \
	      $(DESTDIR)$(PREFIX)/share/man/man1/wj.1 \
	      $(DESTDIR)$(PREFIX)/share/doc/wj/wj.cfg.example

clean:
	rm -f tui/wj-tui
