#!/usr/bin/env bash
# Release wj end-to-end: bump version, tag on GitHub, smoke-test the package,
# and publish to the AUR.
#
#   Usage:  packaging/aur/release.sh <version>     e.g.  release.sh 0.12.0
#
# Runnable from any directory: paths are derived from the script's location and
# every git/build step uses `git -C` or a `( cd … )` subshell, so your current
# shell directory is never changed.
set -euo pipefail

V="${1:?usage: release.sh <version>   (e.g. 0.12.0)}"

# Resolve key locations from where this script lives.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel)"
AUR_SRC="$REPO/packaging/aur"          # source PKGBUILD lives here
AUR_REPO="${WJ_AUR_REPO:-/tmp/wj-aur}" # clone of ssh://aur@aur.archlinux.org/wj.git
PKGBUILD="$AUR_SRC/PKGBUILD"

# Safety: this script commits to the CURRENT branch but pushes main, so it must
# run from main; and `git commit -am` would sweep any stray edits into the
# release commit, so the tree must be clean before we bump.
BR="$(git -C "$REPO" symbolic-ref --short HEAD 2>/dev/null || true)"
[ "$BR" = main ] || { echo "release.sh: run from 'main' (currently on '$BR')" >&2; exit 1; }
git -C "$REPO" diff --quiet && git -C "$REPO" diff --cached --quiet \
    || { echo "release.sh: working tree not clean — commit or stash first" >&2; exit 1; }

echo "==> Releasing wj $V"

# 1. Bump the version everywhere it is recorded: the CLI's WJ_VERSION, the man
#    page's .TH line (keeping its year), and the PKGBUILD pkgver (reset pkgrel).
sed -i "s/^WJ_VERSION=.*/WJ_VERSION=\"$V\"/"             "$REPO/wj"
sed -i "1s/\"wj [0-9][0-9.]*\"/\"wj $V\"/"               "$REPO/wj.1"
sed -i "s/^pkgver=.*/pkgver=$V/; s/^pkgrel=.*/pkgrel=1/" "$PKGBUILD"

# 2. Commit the bump, push, and tag the release so GitHub builds the tarball.
git -C "$REPO" commit -am "Release $V"
git -C "$REPO" push origin main
git -C "$REPO" tag -a "v$V" -m "wj $V"
git -C "$REPO" push origin "v$V"

# 3. Pin the real checksum (needs the tag live on GitHub) and smoke-test build.
( cd "$AUR_SRC" && updpkgsums && makepkg -si )

# 4. Commit the pinned checksum.
git -C "$REPO" commit -am "Release $V: pin checksum"
git -C "$REPO" push origin main

# 5. Publish to the AUR repo (clone it on first use if missing).
if [ ! -d "$AUR_REPO/.git" ]; then
    git clone ssh://aur@aur.archlinux.org/wj.git "$AUR_REPO"
fi
cp "$PKGBUILD" "$AUR_REPO/"
( cd "$AUR_REPO" && makepkg --printsrcinfo > .SRCINFO )
git -C "$AUR_REPO" add PKGBUILD .SRCINFO
git -C "$AUR_REPO" commit -m "Update to $V"
git -C "$AUR_REPO" push

echo "==> Done. wj $V released — https://aur.archlinux.org/packages/wj"
