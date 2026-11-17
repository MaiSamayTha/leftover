#!/bin/sh
# Install Leftover on macOS or Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/MaiSamayTha/leftover/main/scripts/install.sh | sh
#
# What it does, so you don't have to read the rest:
#   1. downloads the latest release for your OS and CPU from GitHub
#   2. checks it against the release's published SHA-256 checksums
#   3. puts the leftover command in ~/.local/bin (no sudo)
#   4. adds a launcher: ~/Applications/Leftover.app on macOS, a menu entry on Linux
#
# Set LEFTOVER_VERSION=v0.1.0 to pick a version, LEFTOVER_BIN_DIR to choose
# where the command goes, or pass --uninstall to remove everything again.
set -eu

REPO="MaiSamayTha/leftover"
BIN_DIR="${LEFTOVER_BIN_DIR:-$HOME/.local/bin}"
VERSION="${LEFTOVER_VERSION:-latest}"

say() { printf '%s\n' "$*"; }
die() { printf 'leftover install: %s\n' "$*" >&2; exit 1; }

os=$(uname -s)
case "$os" in
  Darwin) os=macos ;;
  Linux) os=linux ;;
  *) die "unsupported system $os (use install.ps1 on Windows)" ;;
esac

uninstall() {
  rm -f "$BIN_DIR/leftover"
  rm -rf "$HOME/Applications/Leftover.app"
  rm -f "$HOME/.local/share/applications/leftover.desktop" "$HOME/.local/share/icons/hicolor/scalable/apps/leftover.svg"
  say "Leftover is uninstalled. Its own data folder (settings, history and any vault) was left in place;"
  say "run 'leftover where' before uninstalling next time to find it, or delete it yourself."
  exit 0
}
[ "${1:-}" = "--uninstall" ] && uninstall

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported CPU $arch" ;;
esac
# Rosetta reports x86_64 on Apple silicon; prefer the native build.
if [ "$os" = macos ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
  arch=arm64
fi

if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL --proto '=https' --tlsv1.2 -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q --https-only -O "$2" "$1"; }
else
  die "needs curl or wget"
fi

if command -v sha256sum >/dev/null 2>&1; then
  sha() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
  sha() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
  die "needs sha256sum or shasum to verify the download"
fi

if [ "$VERSION" = latest ]; then
  base="https://github.com/$REPO/releases/latest/download"
else
  base="https://github.com/$REPO/releases/download/$VERSION"
fi
asset="leftover-$os-$arch.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

say "Downloading $asset…"
fetch "$base/$asset" "$tmp/$asset" || die "couldn't download $base/$asset"
fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "couldn't download checksums.txt"

want=$(awk -v f="$asset" '$2 == f || $2 == "*"f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || die "$asset isn't listed in checksums.txt"
got=$(sha "$tmp/$asset")
[ "$want" = "$got" ] || die "checksum mismatch for $asset (expected $want, got $got). Nothing was installed."
say "Checksum verified."

tar -xzf "$tmp/$asset" -C "$tmp"
[ -f "$tmp/leftover" ] || die "archive didn't contain the leftover binary"
mkdir -p "$BIN_DIR"
install -m 0755 "$tmp/leftover" "$BIN_DIR/leftover"

if [ "$os" = macos ]; then
  app="$HOME/Applications/Leftover.app"
  rm -rf "$app"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
  cat > "$app/Contents/MacOS/Leftover" <<EOF
#!/bin/sh
exec "$BIN_DIR/leftover" app
EOF
  chmod 0755 "$app/Contents/MacOS/Leftover"
  [ -f "$tmp/leftover.icns" ] && cp "$tmp/leftover.icns" "$app/Contents/Resources/leftover.icns"
  cat > "$app/Contents/Info.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>Leftover</string>
  <key>CFBundleDisplayName</key><string>Leftover</string>
  <key>CFBundleIdentifier</key><string>app.leftover.launcher</string>
  <key>CFBundleExecutable</key><string>Leftover</string>
  <key>CFBundleIconFile</key><string>leftover</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSUIElement</key><true/>
</dict>
</plist>
EOF
  say "Added Leftover to ~/Applications."
else
  apps="$HOME/.local/share/applications"
  icons="$HOME/.local/share/icons/hicolor/scalable/apps"
  mkdir -p "$apps" "$icons"
  [ -f "$tmp/leftover.svg" ] && cp "$tmp/leftover.svg" "$icons/leftover.svg"
  cat > "$apps/leftover.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=Leftover
Comment=See what your dev tools left behind and clear it safely
Exec=$BIN_DIR/leftover app
Icon=leftover
Terminal=false
Categories=Utility;Development;
EOF
  say "Added Leftover to your app menu."
fi

say ""
say "Installed $("$BIN_DIR/leftover" version)."
case ":$PATH:" in
  *":$BIN_DIR:"*) say "Run 'leftover' to open the app, or 'leftover scan' in the terminal." ;;
  *) say "Add $BIN_DIR to your PATH to use the leftover command, e.g.:"
     say "  echo 'export PATH=\"$BIN_DIR:\$PATH\"' >> ~/.profile" ;;
esac
