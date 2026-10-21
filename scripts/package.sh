#!/usr/bin/env bash
# Builds the Highlighter desktop app (Tauri) installer for one platform:
#
#   scripts/package.sh macos-arm64   [version]  -> dist/Highlighter-<version>-macos-apple-silicon.zip
#   scripts/package.sh macos-amd64   [version]  -> dist/Highlighter-<version>-macos-intel.zip
#   scripts/package.sh windows-amd64 [version]  -> dist/Highlighter-<version>-windows-x64.zip
#
# Each zip holds the installer (a .dmg, or a -setup.exe) and INSTALL.txt.
# The app bundles the highlighter server and yt-dlp, ffmpeg and deno (yt-dlp's
# JavaScript runtime for YouTube) as Tauri sidecars, so users install nothing
# else. Downloads are checksum-verified and cached in dist/cache. Build macOS
# targets on a Mac and the Windows target on Windows (Tauri does not
# cross-compile installers). version must be semver (from a v* tag); without
# it the version in desktop/src-tauri/tauri.conf.json is used.
# Set SKIP_WEB=1 to reuse an existing web build.
#
# With TAURI_SIGNING_PRIVATE_KEY set (the updater key; CI has it as a secret)
# the build also writes the in-app updater's files into dist/updater/: the
# signed .app.tar.gz (macOS) or setup .exe (Windows) and its .sig, which the
# release job lists in latest.json. Without the key they are skipped.
set -euo pipefail

target=${1:?usage: scripts/package.sh <macos-arm64|macos-amd64|windows-amd64> [version]}
version=${2:-}
version=${version#v}

root=$(cd "$(dirname "$0")/.." && pwd)
dist="$root/dist"
cache="$dist/cache"
tauri="$root/desktop/src-tauri"
mkdir -p "$cache"

# Pinned versions (yt-dlp is always the latest). The ffmpeg hashes come from
# the .sha256 files published next to each archive.
DENO_VERSION=v2.9.7
DMGBUILD_VERSION=1.6.7
FFMPEG_MAC_ARM64_URL=https://ffmpeg.martin-riedl.de/download/macos/arm64/1789931890_9.0.2/ffmpeg.zip
FFMPEG_MAC_ARM64_SHA=c8ed4c4e6978a03c485edbfe4e0a5dc2380f8a30bba5150531b31b094492d924
FFMPEG_MAC_AMD64_URL=https://ffmpeg.martin-riedl.de/download/macos/amd64/1789931006_9.0.2/ffmpeg.zip
FFMPEG_MAC_AMD64_SHA=7c6b4125b191cbf773832dc51f424cf2b6bb7da43007d1e066f95909e47cacd4
FFMPEG_WIN_URL=https://www.gyan.dev/ffmpeg/builds/packages/ffmpeg-9.0.2-essentials_build.zip
FFMPEG_WIN_SHA=60f467265b1e312373dbcd92200c2618a74850f98d3d078e94296bb3fa2047ba

case "$target" in
  macos-arm64)
    goos=darwin goarch=arm64 label=macos-apple-silicon triple=aarch64-apple-darwin bundle=app
    ytdlp_asset=yt-dlp_macos deno_asset=deno-aarch64-apple-darwin.zip
    ffmpeg_url=$FFMPEG_MAC_ARM64_URL ffmpeg_sha=$FFMPEG_MAC_ARM64_SHA ;;
  macos-amd64)
    goos=darwin goarch=amd64 label=macos-intel triple=x86_64-apple-darwin bundle=app
    ytdlp_asset=yt-dlp_macos deno_asset=deno-x86_64-apple-darwin.zip
    ffmpeg_url=$FFMPEG_MAC_AMD64_URL ffmpeg_sha=$FFMPEG_MAC_AMD64_SHA ;;
  windows-amd64)
    goos=windows goarch=amd64 label=windows-x64 triple=x86_64-pc-windows-msvc bundle=nsis
    ytdlp_asset=yt-dlp.exe deno_asset=deno-x86_64-pc-windows-msvc.zip
    ffmpeg_url=$FFMPEG_WIN_URL ffmpeg_sha=$FFMPEG_WIN_SHA ;;
  *) echo "unknown target: $target" >&2; exit 2 ;;
esac
exe=""; [[ $goos == windows ]] && exe=.exe

sha256() {
  if command -v shasum >/dev/null; then shasum -a 256 "$1"; else sha256sum "$1"; fi | cut -d' ' -f1
}

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

# Tauri sidecars are named <name>-<target triple>; the bundler strips the
# suffix and places them next to the app's executable.
bins="$tauri/binaries"
rm -rf "$bins" && mkdir -p "$bins" "$tauri/resources"

echo "building highlighter-server for $goos/$goarch"
(cd "$root" && CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath \
  -ldflags "-s -w -X main.version=${version:-dev}" -o "$bins/highlighter-server-$triple$exe" ./cmd/highlighter)

cp "$tools_cache/$ytdlp_tag-$ytdlp_asset" "$bins/yt-dlp-$triple$exe"
unzip -q -o "$tools_cache/$DENO_VERSION-$deno_asset" -d "$work/deno"
cp "$work/deno/deno$exe" "$bins/deno-$triple$exe"
unzip -q -o "$tools_cache/ffmpeg-$ffmpeg_sha.zip" -d "$work/ffmpeg"
cp "$(find "$work/ffmpeg" -type f -name "ffmpeg$exe" | head -1)" "$bins/ffmpeg-$triple$exe"
chmod +x "$bins"/*
cp "$cache/cacert.pem" "$tauri/resources/cacert.pem"

pnpm --dir "$root/desktop" install --frozen-lockfile
rustup target add "$triple" >/dev/null 2>&1 || true
args=(build --target "$triple" --bundles "$bundle")
config=()
[[ -n $version ]] && config+=("\"version\":\"$version\"")
[[ -z ${TAURI_SIGNING_PRIVATE_KEY:-} ]] && config+=('"bundle":{"createUpdaterArtifacts":false}')
if (( ${#config[@]} )); then
  args+=(--config "{$(IFS=,; echo "${config[*]}")}")
fi
pnpm --dir "$root/desktop" tauri "${args[@]}"

out_dir="$tauri/target/$triple/release/bundle/$([[ $bundle == app ]] && echo macos || echo "$bundle")"
name="Highlighter-${version:-$(git -C "$root" describe --tags --always --dirty 2>/dev/null || echo dev)}-$label"
pkg="$work/$name"
mkdir -p "$pkg" "$dist"
if [[ $bundle == app ]]; then
  # The .dmg is built with dmgbuild rather than Tauri: Tauri styles the
  # window by scripting Finder, which CI skips, leaving a bare Finder window.
  # dmgbuild writes the layout (background, arrow, icon places) directly and
  # compresses with LZMA, about 17% smaller than Tauri's zlib .dmg.
  venv="$cache/dmgbuild-$DMGBUILD_VERSION"
  if [[ ! -x $venv/bin/dmgbuild ]]; then
    python3 -m venv "$venv"
    "$venv/bin/pip" install --quiet "dmgbuild==$DMGBUILD_VERSION"
  fi
  "$venv/bin/dmgbuild" -s "$root/packaging/macos/dmg-settings.py" \
    -D app="$out_dir/Highlighter.app" \
    -D background="$root/packaging/macos/dmg-background.tiff" \
    -D volume_icon="$tauri/icons/icon.icns" \
    "Highlighter" "$pkg/$name.dmg"
  cp "$root/packaging/macos/INSTALL.txt" "$pkg/"
else
  cp "$(ls -t "$out_dir"/*-setup.exe | head -1)" "$pkg/$name-setup.exe"
  cp "$root/packaging/windows/INSTALL.txt" "$pkg/"
fi
# Updater files, named per platform so all three fit in one release.
if [[ -n ${TAURI_SIGNING_PRIVATE_KEY:-} ]]; then
  mkdir -p "$dist/updater"
  if [[ $bundle == app ]]; then
    cp "$out_dir/Highlighter.app.tar.gz" "$dist/updater/$name.app.tar.gz"
    cp "$out_dir/Highlighter.app.tar.gz.sig" "$dist/updater/$name.app.tar.gz.sig"
  else
    setup=$(ls -t "$out_dir"/*-setup.exe | head -1)
    cp "$setup" "$dist/updater/$name-setup.exe"
    cp "$setup.sig" "$dist/updater/$name-setup.exe.sig"
  fi
fi

out="$dist/$name.zip"
rm -f "$out"
if command -v zip >/dev/null; then
  (cd "$work" && zip -q -r -9 "$out" "$name")
else # Git Bash on Windows has no zip; GitHub's Windows runners have 7-Zip
  (cd "$work" && 7z a -tzip -mx=9 "$(cygpath -w "$out")" "$name" >/dev/null)
fi

echo "built $out ($(du -h "$out" | cut -f1)) with yt-dlp $ytdlp_tag, deno $DENO_VERSION, ffmpeg 9.0.2"
