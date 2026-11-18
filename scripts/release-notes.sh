#!/bin/sh
# Prints one version's section of CHANGELOG.md, for the GitHub release notes.
# Usage: release-notes.sh 0.1.0 [CHANGELOG.md]
set -eu
version=${1#v}
file=${2:-CHANGELOG.md}
notes=$(awk -v v="$version" '
  /^## \[/ { if (on) exit; if (index($0, "## [" v "]") == 1) { on = 1; next } }
  on { print }
' "$file" | sed -e '/./,$!d')
[ -n "$notes" ] || { echo "release-notes: no section for $version in $file" >&2; exit 1; }
printf '%s\n\n' "$notes"
echo "Verify downloads against \`checksums.txt\`, or use the install scripts, which do it for you."
