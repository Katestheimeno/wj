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

echo "==> Releasing wj $V"

# 1. Bump pkgver (and reset pkgrel) in the source PKGBUILD.
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
