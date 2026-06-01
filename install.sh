#!/usr/bin/env bash
#
# wj installer — fetches the `wj` script from GitHub and drops it on your PATH.
#
#   Install:    curl -fsSL https://raw.githubusercontent.com/Katestheimeno/wj/main/install.sh | bash
#   Uninstall:  curl -fsSL https://raw.githubusercontent.com/Katestheimeno/wj/main/install.sh | bash -s -- --uninstall
#   Purge all:  curl -fsSL https://raw.githubusercontent.com/Katestheimeno/wj/main/install.sh | bash -s -- --uninstall --purge
#
# Options:
#   --with-ui         Also build & install the optional `wj-tui` front-end
#                     (requires Go; the bash CLI never needs it).
#   --uninstall, -u   Remove the wj binary (keeps your config & data).
#   --purge           With --uninstall, also delete config and data dirs.
#   --help, -h        Show this help.
#
# Environment:
#   WJ_BIN_DIR   Where to install/remove the binary (default: ~/.local/bin)
#   WJ_REF       Git ref/branch to install from (default: main)
#   WJ_WITH_UI   Set to 1 for the same effect as --with-ui
#
set -euo pipefail

REPO="Katestheimeno/wj"
REF="${WJ_REF:-main}"
RAW_BASE="${WJ_RAW_BASE:-https://raw.githubusercontent.com/$REPO/$REF}"
BIN_NAME="wj"
TUI_NAME="wj-tui"
BIN_DIR="${WJ_BIN_DIR:-$HOME/.local/bin}"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/wj"
DATA_DIR="${WJ_DATA_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/wj}"
MAN_DIR="${WJ_MAN_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/man/man1}"
TARBALL_URL="${WJ_TARBALL_URL:-https://github.com/$REPO/archive/$REF.tar.gz}"

ACTION=install
PURGE=0
WITH_UI="${WJ_WITH_UI:-0}"
TMPFILE=""
TMPDIR_BUILD=""
trap 'rm -f "$TMPFILE"; [ -n "$TMPDIR_BUILD" ] && rm -rf "$TMPDIR_BUILD"' EXIT

say()  { printf '%s\n' "$*"; }
warn() { printf 'wj-install: %s\n' "$*" >&2; }
die()  { printf 'wj-install: %s\n' "$*" >&2; exit 1; }

usage() {
    cat <<'EOF'
wj installer — fetch the `wj` script from GitHub and put it on your PATH.

USAGE
  install.sh [--with-ui] [--uninstall] [--purge] [--help]

OPTIONS
  --with-ui         Also build & install the wj-tui front-end (needs Go)
  --uninstall, -u   Remove the wj (and wj-tui) binaries (keeps config & data)
  --purge           With --uninstall, also delete config and data dirs
  --help, -h        Show this help

ENVIRONMENT
  WJ_BIN_DIR        Where to install/remove the binary (default: ~/.local/bin)
  WJ_REF            Git ref/branch to install from (default: main)
  WJ_WITH_UI        Set to 1 for the same effect as --with-ui

EXAMPLES
  curl -fsSL https://raw.githubusercontent.com/Katestheimeno/wj/main/install.sh | bash
  curl -fsSL https://raw.githubusercontent.com/Katestheimeno/wj/main/install.sh | bash -s -- --with-ui
  curl -fsSL https://raw.githubusercontent.com/Katestheimeno/wj/main/install.sh | bash -s -- --uninstall
EOF
}

for a in "$@"; do
    case "$a" in
        --with-ui)      WITH_UI=1 ;;
        --uninstall|-u) ACTION=uninstall ;;
        --purge)        PURGE=1 ;;
        --help|-h)      usage; exit 0 ;;
        *)              die "unknown option: $a (try --help)" ;;
    esac
done

fetch() { # url -> stdout
    if command -v curl >/dev/null 2>&1; then curl -fsSL "$1"
    elif command -v wget >/dev/null 2>&1; then wget -qO- "$1"
    else die "need curl or wget to download"; fi
}

do_install() {
    mkdir -p "$BIN_DIR"
    TMPFILE=$(mktemp)
    say "Downloading wj ($REF) from github.com/$REPO ..."
    fetch "$RAW_BASE/$BIN_NAME" > "$TMPFILE" || die "download failed"
    head -n1 "$TMPFILE" | grep -q '^#!' || die "downloaded file does not look like a script"
    cp "$TMPFILE" "$BIN_DIR/$BIN_NAME"
    chmod 0755 "$BIN_DIR/$BIN_NAME"
    say "Installed -> $BIN_DIR/$BIN_NAME"
    # man page (best-effort)
    if fetch "$RAW_BASE/wj.1" > "$TMPFILE" 2>/dev/null && head -n1 "$TMPFILE" | grep -q '^\.TH'; then
        mkdir -p "$MAN_DIR" && cp "$TMPFILE" "$MAN_DIR/wj.1" && say "Installed -> $MAN_DIR/wj.1"
    fi
    [ "$WITH_UI" -eq 1 ] && install_ui
    case ":$PATH:" in
        *":$BIN_DIR:"*) ;;
        *) warn "$BIN_DIR is not on your PATH. Add this to your shell rc:"
           warn "  export PATH=\"$BIN_DIR:\$PATH\"" ;;
    esac
    say "Done. Run 'wj help' to get started."
    say "Tip: shell completion -> add 'eval \"\$(wj completion bash)\"' to your shell rc (or 'completion zsh')."
}

# build & install the optional wj-tui front-end from the source tarball.
install_ui() {
    command -v go >/dev/null 2>&1 || die "--with-ui needs Go on your PATH (https://go.dev/dl)"
    command -v tar >/dev/null 2>&1 || die "--with-ui needs tar to unpack the source"
    TMPDIR_BUILD=$(mktemp -d)
    say "Building $TUI_NAME ($REF) — fetching source ..."
    fetch "$TARBALL_URL" | tar -xz -C "$TMPDIR_BUILD" || die "could not fetch/unpack source tarball"
    local tuidir
    tuidir=$(find "$TMPDIR_BUILD" -maxdepth 2 -type d -name tui -print -quit)
    [ -n "$tuidir" ] || die "source tarball has no tui/ directory (need ref with the UI)"
    say "Compiling (this may take a moment) ..."
    ( cd "$tuidir" && CGO_ENABLED=0 go build -trimpath \
        -ldflags="-s -w -X main.version=$REF" -o "$BIN_DIR/$TUI_NAME" . ) \
        || die "go build failed"
    chmod 0755 "$BIN_DIR/$TUI_NAME"
    say "Installed -> $BIN_DIR/$TUI_NAME"
    say "Enable it: set 'interface=ui' in your config, then run 'wj' (or 'wj ui')."
}

do_uninstall() {
    if [ -e "$BIN_DIR/$BIN_NAME" ] || [ -L "$BIN_DIR/$BIN_NAME" ]; then
        rm -f "$BIN_DIR/$BIN_NAME"; say "Removed $BIN_DIR/$BIN_NAME"
    else
        say "No wj binary in $BIN_DIR (set WJ_BIN_DIR if you installed elsewhere)."
    fi
    if [ -e "$BIN_DIR/$TUI_NAME" ] || [ -L "$BIN_DIR/$TUI_NAME" ]; then
        rm -f "$BIN_DIR/$TUI_NAME"; say "Removed $BIN_DIR/$TUI_NAME"
    fi
    [ -e "$MAN_DIR/wj.1" ] && { rm -f "$MAN_DIR/wj.1"; say "Removed $MAN_DIR/wj.1"; }
    # Warn about — but never auto-delete — copies elsewhere on PATH.
    local other; other=$(command -v "$BIN_NAME" 2>/dev/null || true)
    if [ -n "$other" ] && [ "$other" != "$BIN_DIR/$BIN_NAME" ]; then
        warn "Another wj is still on your PATH at $other — remove it manually if unwanted."
    fi
    if [ "$PURGE" -eq 1 ]; then
        rm -rf "$CONFIG_DIR" "$DATA_DIR"
        say "Purged config ($CONFIG_DIR) and data ($DATA_DIR)"
    else
        say "Kept your config ($CONFIG_DIR) and data ($DATA_DIR)."
        say "Re-run with --purge to delete them too."
    fi
}

case "$ACTION" in
    install)   do_install ;;
    uninstall) do_uninstall ;;
esac
