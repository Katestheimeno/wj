# Publishing `wj` to the AUR (tag-based releases)

The AUR package lives in its **own git repo** on `aur.archlinux.org`, separate
from this GitHub repo. This folder holds the source `PKGBUILD`; you copy it into
the AUR repo and push from there.

## One-time setup

1. Create an account at https://aur.archlinux.org and add your **SSH public key**
   under *My Account* (the AUR only accepts pushes over SSH).
2. Install the tooling locally:
   ```sh
   sudo pacman -S --needed base-devel git namcap pacman-contrib
   ```
   (`pacman-contrib` provides `updpkgsums`.)

## Releasing a version

The flow for every release (first one is `v0.1.0`):

1. **Tag and push** from this repo so GitHub generates the release tarball:
   ```sh
   git tag -a v0.1.0 -m "wj 0.1.0"
   git push origin v0.1.0
   ```

2. **Pin the checksum.** From this folder, with the tag now live on GitHub:
   ```sh
   cd packaging/aur
   updpkgsums          # replaces sha256sums=('SKIP') with the real hash
   ```

3. **Test the build** in a clean dir:
   ```sh
   makepkg -si         # builds, then installs; confirm `wj help` works
   namcap PKGBUILD
   namcap wj-*.pkg.tar.zst
   ```

4. **Publish to the AUR.** First release = initial import:
   ```sh
   git clone ssh://aur@aur.archlinux.org/wj.git /tmp/wj-aur
   cp PKGBUILD /tmp/wj-aur/
   cd /tmp/wj-aur
   makepkg --printsrcinfo > .SRCINFO   # REQUIRED — AUR rejects pushes without it
   git add PKGBUILD .SRCINFO
   git commit -m "Initial import: wj 0.1.0"
   git push
   ```

   Package page: https://aur.archlinux.org/packages/wj — installable with
   `yay -S wj` / `paru -S wj`.

## Bumping to a later version

1. Edit `pkgver` here (reset `pkgrel=1`), tag & push the new tag on GitHub.
2. `updpkgsums` to refresh the checksum.
3. In the AUR clone: copy the new `PKGBUILD`, regenerate `.SRCINFO`, commit, push.

Bump only `pkgrel` (e.g. `1` → `2`) when you change packaging but not the
upstream version.
