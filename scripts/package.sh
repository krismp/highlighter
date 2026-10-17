#!/usr/bin/env bash
# Builds a self-contained, ready-to-install highlighter for one platform:
#
#   scripts/package.sh macos-arm64   [version]  -> dist/Highlighter-<version>-macos-apple-silicon.dmg
#   scripts/package.sh macos-amd64   [version]  -> dist/Highlighter-<version>-macos-intel.dmg
#   scripts/package.sh windows-amd64 [version]  -> dist/Highlighter-<version>-windows-x64.zip
#
# The package bundles yt-dlp, ffmpeg and deno (yt-dlp's JavaScript runtime for
# YouTube), so users install nothing else. Downloads are checksum-verified and
# cached in dist/cache. macOS targets must be built on a Mac (codesign, hdiutil).
# Set SKIP_WEB=1 to reuse an existing web build.
set -euo pipefail

target=${1:?usage: scripts/package.sh <macos-arm64|macos-amd64|windows-amd64> [version]}
version=${2:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
version=${version#v}

root=$(cd "$(dirname "$0")/.." && pwd)
dist="$root/dist"
cache="$dist/cache"
mkdir -p "$cache"

# Pinned versions (yt-dlp is always the latest). The ffmpeg hashes come from
# the .sha256 files published next to each archive.
DENO_VERSION=v2.9.7
FFMPEG_MAC_ARM64_URL=https://ffmpeg.martin-riedl.de/download/macos/arm64/1789931890_9.0.2/ffmpeg.zip
FFMPEG_MAC_ARM64_SHA=c8ed4c4e6978a03c485edbfe4e0a5dc2380f8a30bba5150531b31b094492d924
FFMPEG_MAC_AMD64_URL=https://ffmpeg.martin-riedl.de/download/macos/amd64/1789931006_9.0.2/ffmpeg.zip
FFMPEG_MAC_AMD64_SHA=7c6b4125b191cbf773832dc51f424cf2b6bb7da43007d1e066f95909e47cacd4
FFMPEG_WIN_URL=https://www.gyan.dev/ffmpeg/builds/packages/ffmpeg-9.0.2-essentials_build.zip
FFMPEG_WIN_SHA=60f467265b1e312373dbcd92200c2618a74850f98d3d078e94296bb3fa2047ba

case "$target" in
  macos-arm64)
    goos=darwin goarch=arm64 label=macos-apple-silicon
    ytdlp_asset=yt-dlp_macos deno_asset=deno-aarch64-apple-darwin.zip
    ffmpeg_url=$FFMPEG_MAC_ARM64_URL ffmpeg_sha=$FFMPEG_MAC_ARM64_SHA ;;
  macos-amd64)
    goos=darwin goarch=amd64 label=macos-intel
    ytdlp_asset=yt-dlp_macos deno_asset=deno-x86_64-apple-darwin.zip
    ffmpeg_url=$FFMPEG_MAC_AMD64_URL ffmpeg_sha=$FFMPEG_MAC_AMD64_SHA ;;
  windows-amd64)
    goos=windows goarch=amd64 label=windows-x64
    ytdlp_asset=yt-dlp.exe deno_asset=deno-x86_64-pc-windows-msvc.zip
    ffmpeg_url=$FFMPEG_WIN_URL ffmpeg_sha=$FFMPEG_WIN_SHA ;;
  *) echo "unknown target: $target" >&2; exit 2 ;;
esac
exe=""; [[ $goos == windows ]] && exe=.exe

sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }

# fetch URL DEST SHA256: download once into the cache and verify.
fetch() {
  local url=$1 dest=$2 want=$3
  if [[ ! -f $dest ]] || [[ $(sha256 "$dest") != "$want" ]]; then
    echo "downloading $url"
    curl -fL --retry 3 -o "$dest.part" "$url"
    mv "$dest.part" "$dest"
  fi
  local got; got=$(sha256 "$dest")
  if [[ $got != "$want" ]]; then
    echo "checksum mismatch for $url: got $got, want $want" >&2
    rm -f "$dest"; exit 1
  fi
}

# hash_at URL: the first SHA-256 in a checksum file, whatever its layout
# (sha256sum lines, or PowerShell Get-FileHash tables for Deno's Windows build).
hash_at() {
  curl -fsSL --retry 3 "$1" | grep -oE '[0-9A-Fa-f]{64}' | head -1 | tr 'A-F' 'a-f'
}

# checksum_from SUMS_URL FILE: the expected hash of FILE from a release's sums file.
checksum_from() {
  curl -fsSL --retry 3 "$1" | awk -v f="$2" '$2 == f || $2 == "*"f { print $1 }' | head -1
}

tools_cache="$cache/$target"
mkdir -p "$tools_cache"

# yt-dlp: always the latest release; YouTube changes break older ones quickly.
ytdlp_tag=$(curl -fsSI https://github.com/yt-dlp/yt-dlp/releases/latest | tr -d '\r' | awk -F/ 'tolower($1) ~ /^location:/ { print $NF }')
ytdlp_base=https://github.com/yt-dlp/yt-dlp/releases/download/$ytdlp_tag
fetch "$ytdlp_base/$ytdlp_asset" "$tools_cache/$ytdlp_tag-$ytdlp_asset" \
  "$(checksum_from "$ytdlp_base/SHA2-256SUMS" "$ytdlp_asset")"

deno_base=https://github.com/denoland/deno/releases/download/$DENO_VERSION
fetch "$deno_base/$deno_asset" "$tools_cache/$DENO_VERSION-$deno_asset" \
  "$(hash_at "$deno_base/$deno_asset.sha256sum")"

fetch "$ffmpeg_url" "$tools_cache/ffmpeg-$ffmpeg_sha.zip" "$ffmpeg_sha"

# Static ffmpeg builds cannot read the OS certificate store, so HTTPS needs a
# CA bundle: Mozilla's, as packaged by curl.
fetch https://curl.se/ca/cacert.pem "$cache/cacert.pem" \
  "$(hash_at https://curl.se/ca/cacert.pem.sha256)"

# --- build -------------------------------------------------------------------
if [[ ${SKIP_WEB:-} != 1 ]]; then
  make -C "$root" web
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

if [[ $goos == darwin ]]; then
  stage="$work/stage"
  app="$stage/Highlighter.app"
  bindir="$app/Contents/MacOS"
else
  stage="$work/Highlighter"
  bindir="$stage"
fi
mkdir -p "$bindir/tools"

echo "building highlighter $version for $goos/$goarch"
(cd "$root" && CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath \
  -ldflags "-s -w -X main.version=$version" -o "$bindir/Highlighter$exe" ./cmd/highlighter)

cp "$tools_cache/$ytdlp_tag-$ytdlp_asset" "$bindir/tools/yt-dlp$exe"
unzip -q -o "$tools_cache/$DENO_VERSION-$deno_asset" -d "$work/deno"
cp "$work/deno/deno$exe" "$bindir/tools/deno$exe"
unzip -q -o "$tools_cache/ffmpeg-$ffmpeg_sha.zip" -d "$work/ffmpeg"
cp "$(find "$work/ffmpeg" -type f -name "ffmpeg$exe" | head -1)" "$bindir/tools/ffmpeg$exe"
chmod +x "$bindir"/Highlighter$exe "$bindir"/tools/*
cp "$cache/cacert.pem" "$bindir/tools/cacert.pem"

notices="$root/packaging/THIRD_PARTY_NOTICES.txt"
mkdir -p "$dist"

if [[ $goos == darwin ]]; then
  sed "s/@VERSION@/$version/g" "$root/packaging/macos/Info.plist" > "$app/Contents/Info.plist"
  mkdir -p "$app/Contents/Resources"
  cp "$notices" "$app/Contents/Resources/"
  # Ad-hoc signature: not notarized, but keeps Gatekeeper's "Open Anyway"
  # path available instead of reporting the app as damaged.
  codesign --force --deep --sign - "$app"
  codesign --verify --deep --strict "$app"
  ln -s /Applications "$stage/Applications"
  cp "$root/packaging/macos/How to install.txt" "$stage/"
  out="$dist/Highlighter-$version-$label.dmg"
  rm -f "$out"
  hdiutil create -quiet -volname "Highlighter" -srcfolder "$stage" -fs HFS+ -format UDZO -ov "$out"
else
  cp "$root/packaging/windows/README.txt" "$stage/README.txt"
  cp "$notices" "$stage/"
  out="$dist/Highlighter-$version-$label.zip"
  rm -f "$out"
  (cd "$work" && zip -q -r -9 "$out" Highlighter)
fi

echo "built $out ($(du -h "$out" | cut -f1)) with yt-dlp $ytdlp_tag, deno $DENO_VERSION, ffmpeg 9.0.2"
