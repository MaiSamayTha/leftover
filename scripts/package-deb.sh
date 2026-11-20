#!/bin/sh
# Builds leftover-linux-<arch>.deb for Debian, Ubuntu and friends: the
# binary in /usr/bin, an app-menu entry and its icon. Double-clicking the
# file opens it in the system's software installer. Needs dpkg-deb.
#
# Usage: package-deb.sh <version> <out-dir> [amd64 arm64 ...]
set -eu
version=${1#v}
out=$2
shift 2
arches=${*:-amd64 arm64}
root=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$out"
out=$(cd "$out" && pwd)

for arch in $arches; do
  work=$(mktemp -d)
  pkg="$work/leftover"
  mkdir -p "$pkg/DEBIAN" "$pkg/usr/bin" "$pkg/usr/share/applications" \
    "$pkg/usr/share/icons/hicolor/scalable/apps" "$pkg/usr/share/doc/leftover" \
    "$pkg/usr/share/bash-completion/completions" "$pkg/usr/share/zsh/vendor-completions" \
    "$pkg/usr/share/fish/vendor_completions.d"
  (cd "$root" && GOOS=linux GOARCH=$arch CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=$version" -o "$pkg/usr/bin/leftover" ./cmd/leftover)
  cp "$root/assets/leftover.svg" "$pkg/usr/share/icons/hicolor/scalable/apps/leftover.svg"
  cp "$root/packaging/linux/leftover.desktop" "$pkg/usr/share/applications/leftover.desktop"
  cp "$root/LICENSE" "$pkg/usr/share/doc/leftover/copyright"
  # Completion scripts are the same on every architecture.
  (cd "$root" && go run ./cmd/leftover completion bash) > "$pkg/usr/share/bash-completion/completions/leftover"
  (cd "$root" && go run ./cmd/leftover completion zsh) > "$pkg/usr/share/zsh/vendor-completions/_leftover"
  (cd "$root" && go run ./cmd/leftover completion fish) > "$pkg/usr/share/fish/vendor_completions.d/leftover.fish"
  size=$(du -sk "$pkg/usr" | cut -f1)
  cat > "$pkg/DEBIAN/control" <<EOF
Package: leftover
Version: $version
Architecture: $arch
Maintainer: Leftover contributors <129627567+MaiSamayTha@users.noreply.github.com>
Installed-Size: $size
Section: utils
Priority: optional
Homepage: https://maisamaytha.github.io/leftover/
Description: find and safely clear what your dev tools leave behind
 Leftover explains every cache, build folder and AI model your developer
 tools leave on disk, and moves what's safe into a vault you can restore
 from for 14 days instead of deleting it. Runs entirely on your computer.
EOF
  chmod 0755 "$pkg/usr/bin/leftover"
  find "$pkg" -type d -exec chmod 0755 {} +
  dpkg-deb --root-owner-group --build "$pkg" "$out/leftover-linux-$arch.deb" >/dev/null
  rm -rf "$work"
  echo "package-deb: wrote $out/leftover-linux-$arch.deb"
done
