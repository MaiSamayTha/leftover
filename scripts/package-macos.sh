#!/bin/sh
# Builds Leftover.app (one universal binary for Apple silicon and Intel) and
# wraps it in Leftover.dmg: open it, drag Leftover onto Applications, done.
# Runs on macOS, which has lipo, codesign and hdiutil.
#
# Usage: package-macos.sh <version> <out-dir>
#
# Signing: without credentials the app is ad-hoc signed, so macOS asks people
# to approve it once (System Settings > Privacy & Security > Open Anyway).
# With these set, it's signed with a Developer ID and notarized instead, and
# opens without any warning:
#   MACOS_SIGN_IDENTITY   e.g. "Developer ID Application: Name (TEAMID)"
#   APPLE_ID, APPLE_APP_PASSWORD, APPLE_TEAM_ID   for notarization
set -eu
version=${1#v}
out=$2
root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$out"

for arch in arm64 amd64; do
  (cd "$root" && GOOS=darwin GOARCH=$arch CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=$version" -o "$work/leftover-$arch" ./cmd/leftover)
done

app="$work/dmg/Leftover.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
lipo -create -output "$app/Contents/MacOS/Leftover" "$work/leftover-arm64" "$work/leftover-amd64"
cp "$root/assets/leftover.icns" "$app/Contents/Resources/Leftover.icns"
sed "s/@VERSION@/$version/g" "$root/packaging/macos/Info.plist" > "$app/Contents/Info.plist"
printf 'APPL????' > "$app/Contents/PkgInfo"

if [ -n "${MACOS_SIGN_IDENTITY:-}" ]; then
  codesign --force --options runtime --timestamp --sign "$MACOS_SIGN_IDENTITY" "$app"
else
  codesign --force --sign - "$app"
fi
codesign --verify --strict "$app"

# The same app, zipped, for Leftover's built-in updater. ditto keeps the
# code signature and permissions intact.
ditto -c -k --keepParent "$app" "$out/Leftover-macos.zip"

ln -s /Applications "$work/dmg/Applications"
dmg="$out/Leftover.dmg"
rm -f "$dmg"
# hdiutil occasionally writes an image that won't mount ("corrupt image"),
# seen on GitHub's macOS runners. Check each one opens, and try again.
ok=
for try in 1 2 3; do
  rm -f "$dmg"
  hdiutil create -quiet -volname "Leftover" -srcfolder "$work/dmg" -fs HFS+ -format UDZO -ov "$dmg" || continue
  check=$(mktemp -d)
  if hdiutil attach -quiet -nobrowse -readonly -mountpoint "$check" "$dmg"; then
    hdiutil detach -quiet "$check" || hdiutil detach -force "$check"
    ok=1
    break
  fi
  echo "package-macos: Leftover.dmg didn't open (try $try of 3); making it again" >&2
  sleep 2
done
[ -n "$ok" ] || { echo "package-macos: couldn't make a Leftover.dmg that opens" >&2; exit 1; }

if [ -n "${MACOS_SIGN_IDENTITY:-}" ]; then
  codesign --force --timestamp --sign "$MACOS_SIGN_IDENTITY" "$dmg"
  if [ -n "${APPLE_ID:-}" ] && [ -n "${APPLE_APP_PASSWORD:-}" ] && [ -n "${APPLE_TEAM_ID:-}" ]; then
    xcrun notarytool submit "$dmg" --apple-id "$APPLE_ID" --password "$APPLE_APP_PASSWORD" --team-id "$APPLE_TEAM_ID" --wait
    xcrun stapler staple "$dmg"
  fi
fi
echo "package-macos: wrote $out/Leftover-macos.zip"
echo "package-macos: wrote $dmg ($(lipo -archs "$app/Contents/MacOS/Leftover"))"
