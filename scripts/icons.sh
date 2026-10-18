#!/usr/bin/env bash
# Regenerates every app icon and installer image from the SVGs (run on a Mac):
#   packaging/icon/icon.svg              -> desktop/src-tauri/icons/ (.icns, .ico, PNGs)
#                                           and web/public/favicon.svg
#   packaging/macos/dmg-background.svg   -> packaging/macos/dmg-background.tiff
#                                           (the .dmg window, normal and Retina)
#   packaging/windows/nsis-*.svg         -> desktop/src-tauri/installer/*.bmp
#                                           (Windows installer header and sidebar)
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

"$work/render" "$root/packaging/macos/dmg-background.svg" "$work/bg.png" 660x400
"$work/render" "$root/packaging/macos/dmg-background.svg" "$work/bg@2x.png" 1320x800
tiffutil -cathidpicheck "$work/bg.png" "$work/bg@2x.png" -out "$root/packaging/macos/dmg-background.tiff"

mkdir -p "$root/desktop/src-tauri/installer"
"$work/render" "$root/packaging/windows/nsis-header.svg" "$root/desktop/src-tauri/installer/nsis-header.bmp" 150x57
"$work/render" "$root/packaging/windows/nsis-sidebar.svg" "$root/desktop/src-tauri/installer/nsis-sidebar.bmp" 164x314
echo "icons regenerated"
