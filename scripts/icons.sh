#!/usr/bin/env bash
# Regenerates every app icon from packaging/icon/icon.svg (run on a Mac):
#   packaging/icon/AppIcon.icns              macOS app and .dmg volume icon
#   cmd/highlighter/rsrc_windows_amd64.syso  icon Go links into Highlighter.exe
#   web/public/favicon.svg                   browser tab
# The outputs are committed, so release builds need none of these tools.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
svg="$root/packaging/icon/icon.svg"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

swiftc -O -o "$work/render" "$root/scripts/render-svg.swift"
set=$work/AppIcon.iconset
mkdir -p "$set"
for s in 16 32 128 256 512; do
  "$work/render" "$svg" "$set/icon_${s}x${s}.png" $s
  "$work/render" "$svg" "$set/icon_${s}x${s}@2x.png" $((s * 2))
done
iconutil -c icns -o "$root/packaging/icon/AppIcon.icns" "$set"

"$work/render" "$svg" "$work/icon.png" 256
(cd "$root" && go run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64 \
  --icon "$work/icon.png" --manifest cli --product-name Highlighter \
  --file-description Highlighter --out cmd/highlighter/rsrc)

cp "$svg" "$root/web/public/favicon.svg"
echo "icons regenerated"
