#!/usr/bin/env bash
# Regenerates every app icon from packaging/icon/icon.svg (run on a Mac):
#   desktop/src-tauri/icons/   macOS .icns, Windows .ico and PNGs for the desktop app
#   web/public/favicon.svg     browser tab
# The outputs are committed, so release builds need none of these tools.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
svg="$root/packaging/icon/icon.svg"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

swiftc -O -o "$work/render" "$root/scripts/render-svg.swift"
"$work/render" "$svg" "$work/icon.png" 1024
pnpm --dir "$root/desktop" install --frozen-lockfile
rm -rf "$root/desktop/src-tauri/icons"
pnpm --dir "$root/desktop" tauri icon "$work/icon.png" -o "$root/desktop/src-tauri/icons"
# Only desktop icons are used; drop the iOS/Android/Windows Store sets.
rm -rf "$root/desktop/src-tauri/icons/"{ios,android} "$root/desktop/src-tauri/icons/"Square*.png "$root/desktop/src-tauri/icons/StoreLogo.png"

cp "$svg" "$root/web/public/favicon.svg"
echo "icons regenerated"
