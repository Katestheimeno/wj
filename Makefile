# wj — the CLI is the pure-bash `wj` script (nothing to build).
# These targets build and install the optional Go TUI front-end, wj-tui.

PREFIX  ?= $(HOME)/.local
BINDIR  ?= $(PREFIX)/bin
GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all tui test vet fmt install install-cli install-ui uninstall clean

all: tui

## tui: build the wj-tui binary into ./tui/wj-tui
tui:
	cd tui && $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o wj-tui .

## test: run the Go test suite (incl. integration tests against ./wj)
test:
	cd tui && $(GO) test ./...

vet:
	cd tui && $(GO) vet ./...

fmt:
	cd tui && $(GO) fmt ./...

## install: install both the bash CLI and the TUI
install: install-cli install-ui

install-cli:
	install -Dm755 wj $(DESTDIR)$(BINDIR)/wj
	install -Dm644 wj.1 $(DESTDIR)$(PREFIX)/share/man/man1/wj.1

install-ui: tui
	install -Dm755 tui/wj-tui $(DESTDIR)$(BINDIR)/wj-tui

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/wj $(DESTDIR)$(BINDIR)/wj-tui $(DESTDIR)$(PREFIX)/share/man/man1/wj.1

clean:
	rm -f tui/wj-tui
