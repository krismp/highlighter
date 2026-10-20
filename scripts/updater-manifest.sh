#!/usr/bin/env bash
# Writes latest.json, the feed the in-app updater reads, for one release:
#
#   scripts/updater-manifest.sh <tag> <updater dir> [notes file] > latest.json
#
# <updater dir> holds what scripts/package.sh put in dist/updater for each
# platform (an update file and its .sig). The URLs point at that tag's
# release assets, so upload those files to the release too. The app fetches
# https://github.com/krismp/highlighter/releases/latest/download/latest.json.
set -euo pipefail

tag=${1:?usage: updater-manifest.sh <tag> <updater dir> [notes file]}
dir=${2:?usage: updater-manifest.sh <tag> <updater dir> [notes file]}
notes_file=${3:-}
base=https://github.com/krismp/highlighter/releases/download/$tag

version=${tag#v} # package.sh names files without the v
notes=""
[[ -n $notes_file && -f $notes_file ]] && notes=$(cat "$notes_file")
[[ -z $notes ]] && notes="Highlighter $version"

platforms='{}'
add() { # add PLATFORM FILE: when FILE and its .sig exist
  local file=$dir/$2
  [[ -f $file && -f $file.sig ]] || return 0
  platforms=$(jq --arg p "$1" --arg sig "$(cat "$file.sig")" --arg url "$base/$2" \
    '. + {($p): {signature: $sig, url: $url}}' <<<"$platforms")
}
add darwin-aarch64 "Highlighter-$version-macos-apple-silicon.app.tar.gz"
add darwin-x86_64 "Highlighter-$version-macos-intel.app.tar.gz"
add windows-x86_64 "Highlighter-$version-windows-x64-setup.exe"

if [[ $platforms == '{}' ]]; then
  echo "no signed update files in $dir" >&2
  exit 1
fi
jq -n --arg v "$version" --arg notes "$notes" --arg date "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --argjson platforms "$platforms" \
  '{version: $v, notes: $notes, pub_date: $date, platforms: $platforms}'
