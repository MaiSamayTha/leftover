#!/bin/sh
# End-to-end check: build the binary, make a fake home folder, and run a full
# scan → clean → restore → empty cycle through the real command line.
# Never touches your actual home folder.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT

bin="$work/leftover"
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) bin="$bin.exe" ;;
esac
(cd "$root" && go build -o "$bin" ./cmd/leftover)

export HOME="$work/home"
export USERPROFILE="$work/home"
export LOCALAPPDATA="$work/home/AppData/Local"
export APPDATA="$work/home/AppData/Roaming"
export XDG_CACHE_HOME="" XDG_DATA_HOME="" PLAYWRIGHT_BROWSERS_PATH="" HF_HOME="" HF_HUB_CACHE=""
export NO_COLOR=1
mkdir -p "$HOME" "$LOCALAPPDATA" "$APPDATA"

mkfile() { mkdir -p "$(dirname "$1")"; head -c "$2" /dev/zero > "$1"; }

mkdir -p "$HOME/code/web" "$HOME/code/engine/target"
echo '{}' > "$HOME/code/web/package.json"
mkfile "$HOME/code/web/node_modules/react/index.js" 200000
touch "$HOME/code/engine/Cargo.toml" "$HOME/code/engine/target/CACHEDIR.TAG"
mkfile "$HOME/code/engine/target/debug/engine" 300000
mkfile "$HOME/.claude/projects/chat.jsonl" 5000
mkfile "$HOME/.cache/huggingface/hub/models--acme--tiny/blob" 100000

fail() { echo "e2e: $*" >&2; exit 1; }
json() { python3 -c "import json,sys; d=json.load(sys.stdin); print($1)"; }

"$bin" scan -q --all --min 0 > "$work/scan.txt"
grep -q "node_modules" "$work/scan.txt" || fail "scan didn't list node_modules"
grep -q "Your data" "$work/scan.txt" || fail "scan didn't list the agent history as your data"

n=$("$bin" scan --json | json "len([i for i in d['items'] if i['verdict'] in ('rebuild','redownload')])")
[ "$n" = 3 ] || fail "expected 3 cleanable items, got $n"

# Without a terminal and without --yes, nothing may move.
"$bin" clean --min 0 < /dev/null > /dev/null
[ -d "$HOME/code/web/node_modules" ] || fail "clean moved things without being asked"

"$bin" clean --yes --json --min 0 > "$work/clean.json"
[ ! -e "$HOME/code/web/node_modules" ] || fail "node_modules still there after clean"
[ ! -e "$HOME/code/engine/target" ] || fail "target still there after clean"
[ -f "$HOME/.claude/projects/chat.jsonl" ] || fail "your data was touched"

id=$("$bin" vault --json | json "[e['id'] for e in d if e['rule']=='node-modules'][0]")
"$bin" vault restore "$id" > /dev/null
[ -f "$HOME/code/web/node_modules/react/index.js" ] || fail "restore didn't bring node_modules back"

"$bin" vault empty --yes > /dev/null
left=$("$bin" vault --json | json "len(d)")
[ "$left" = 0 ] || fail "vault not empty after emptying"

events=$("$bin" history --json -n 0 | json "len(d)")
[ "$events" -ge 5 ] || fail "history should have at least 5 events, has $events"

"$bin" why "$HOME/code/web/node_modules" | grep -q "Downloads again" || fail "why gave the wrong verdict"

echo "e2e: all good"
