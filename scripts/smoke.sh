#!/bin/sh
# Installs the published release with scripts/install.sh into a throwaway
# home folder and uses it like a person would: scan, why, clean, vault,
# restore, empty. Run by CI on macOS and Linux; never touches the real home.
set -eu

work=$(mktemp -d)
trap 'chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT
export HOME="$work/home"
export XDG_CACHE_HOME="" XDG_DATA_HOME="" NO_COLOR=1
mkdir -p "$HOME"
fail() { echo "smoke: $*" >&2; exit 1; }

/bin/sh "$(dirname "$0")/install.sh"
bin="$HOME/.local/bin/leftover"
"$bin" --version | grep -q "^leftover [0-9]" || fail "--version didn't print a version"

mkdir -p "$HOME/code/web/node_modules/react"
echo '{}' > "$HOME/code/web/package.json"
head -c 3000000 /dev/zero > "$HOME/code/web/node_modules/react/index.js"

"$bin" scan -q | tee "$work/scan.txt"
grep -q node_modules "$work/scan.txt" || fail "scan didn't find node_modules"
"$bin" why "$HOME/code/web/node_modules" | grep -q "Downloads again" || fail "why gave the wrong verdict"

"$bin" clean --yes | grep -q "Moved" || fail "clean --yes didn't move anything"
[ ! -e "$HOME/code/web/node_modules" ] || fail "node_modules still there after clean"

id=$("$bin" vault --json | sed -n 's/.*"id": *"\([^"]*\)".*/\1/p' | head -1)
[ -n "$id" ] || fail "nothing in the vault after clean"
"$bin" vault restore "$id" >/dev/null
[ -f "$HOME/code/web/node_modules/react/index.js" ] || fail "restore didn't bring node_modules back"

"$bin" clean --yes >/dev/null
"$bin" vault empty --yes >/dev/null
[ "$("$bin" vault --json | tr -d '[:space:]')" = "[]" ] || fail "vault not empty after emptying"
echo "smoke: installed and used $("$bin" --version) on $(uname -s)/$(uname -m)"
