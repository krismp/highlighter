# highlighter

Paste a YouTube URL, mark the start of each play, review each clip in the
embedded player, and get one MP4 per play. The source aspect ratio is kept.

## Install

Highlighter is a desktop app for macOS and Windows. Download the zip for
your computer from the
[latest release](https://github.com/krismp/highlighter/releases/latest):

| Your computer | Download |
| --- | --- |
| Mac with Apple Silicon (M1 and later) | `Highlighter-<version>-macos-apple-silicon.zip` |
| Mac with an Intel processor | `Highlighter-<version>-macos-intel.zip` |
| Windows 10 or 11, 64-bit | `Highlighter-<version>-windows-x64.zip` |

Not sure which Mac you have? Apple menu → **About This Mac**: "Chip: Apple M…"
is Apple Silicon, "Processor: … Intel" is Intel.

Everything highlighter needs (yt-dlp, ffmpeg and deno) is built into the app,
so there is nothing else to install. It needs macOS 12 or later, or Windows 10
or later. Each zip contains the installer and an `INSTALL.txt` with the steps
below.

### macOS

1. Double-click the downloaded `.zip`, then double-click the `.dmg` inside it.
2. Drag **Highlighter** onto the **Applications** folder.
3. Open **Applications** and double-click **Highlighter**.
4. **First launch only:** the app isn't from the App Store, so macOS says it
   can't verify the developer. Click **Done**, then open **System Settings →
   Privacy & Security**, scroll to *"Highlighter" was blocked…*, click
   **Open Anyway** and confirm with your password or Touch ID.

Highlighter opens in its own window. Close the window, press **Cmd+Q**, or
click **Quit** at the top right to stop it. Clips are saved to
`Movies/Highlights` (changeable in Settings). The log is at
`~/Library/Logs/com.github.krismp.highlighter/server.log`.

If macOS still refuses to open it, run this in Terminal, then open the app again:

```bash
xattr -cr /Applications/Highlighter.app
```

### Windows

1. Right-click the downloaded `.zip` → **Extract All…** → **Extract**.
2. Double-click `Highlighter-<version>-windows-x64-setup.exe`.
3. **First launch only:** if Windows shows *"Windows protected your PC"*,
   click **More info → Run anyway**.
4. Follow the installer. It installs for your account only (no administrator
   rights needed) and adds Highlighter to the Start menu.
5. Start **Highlighter** from the Start menu. Close its window, or click
   **Quit** at the top right, to stop it.

Clips are saved to `Videos\Highlights` (changeable in Settings). The app uses
the Microsoft Edge WebView2 runtime, which Windows 10 and 11 already include;
on systems without it the installer downloads it.

### Good to know

- **Internet:** opening, previewing and cutting YouTube videos needs an
  internet connection. When you're offline, highlighter says so at the top of
  the page and checks again automatically; your saved clips are unaffected.
- **Quitting:** closing the window, **Cmd+Q** (macOS) or **Quit** in the page
  stops highlighter completely; nothing keeps running in the background. If a
  video is still being processed, highlighter asks before quitting.
  Finished clips stay saved; unfinished ones are not saved.
- **Errors:** anything that goes wrong shows up as an alert in the corner of
  the window. **Copy details** copies everything needed for a bug report.
- **Clips folder:** clips go to `Movies/Highlights` on macOS and
  `Videos\Highlights` on Windows. Change it in **Settings**.

### Updating yt-dlp

YouTube changes often break older yt-dlp versions. If videos stop loading,
click **Update yt-dlp** in **Settings**: it downloads the latest yt-dlp into
your profile (`~/.config/highlighter/bin`) and uses it from then on, no new
highlighter download needed.

### Uninstall

macOS: drag `Highlighter.app` from Applications to the Trash. Windows:
**Settings → Apps → Installed apps → Highlighter → Uninstall**.
Saved sessions and settings live in `~/.config/highlighter`; delete that too
for a clean slate. Your clips are never touched.

## Build from source

```bash
brew install go node pnpm yt-dlp ffmpeg deno
make build && ./bin/highlighter   # opens http://127.0.0.1:7777
```

When run from source, highlighter finds yt-dlp, ffmpeg and deno on your PATH.

## Develop

```bash
make dev-api   # Go API on :7777
make dev-web   # Vite on :5173, proxies /api to :7777
make test      # unit tests
make e2e       # clips a real video (network + yt-dlp + ffmpeg)
```

## Desktop app

The desktop app (`desktop/`) is a small [Tauri 2](https://v2.tauri.app) shell.
It starts the Go server as a sidecar on `127.0.0.1` and shows its web UI in a
native window: closing the window stops the server, and **Quit** in the UI
stops the server, which closes the app. yt-dlp, ffmpeg and deno are bundled as
further sidecars, next to the server, and the server looks for them there
first, then on PATH.

## Release

```bash
make dist      # macOS installers (zipped) into dist/; run on a Mac with Rust installed
make smoke     # install the Apple Silicon build in a temp folder and clip a real video
```

`scripts/package.sh <target> [version]` builds one zipped installer
(`macos-arm64`, `macos-amd64` or `windows-amd64`; Windows builds need a
Windows machine). It bundles the latest yt-dlp and pinned, checksum-verified
deno and ffmpeg, builds the server, and runs `tauri build`. The `.dmg` is
recompressed with LZMA, about 17% smaller than Tauri's default.

The app icon is drawn once in `packaging/icon/icon.svg`; after editing it, run
`make icons` (on a Mac) to regenerate the desktop app icons and the browser
favicon, and commit the results.

Pushing a `v*` tag runs `.github/workflows/release.yml`: it builds all three
installers, installs and smoke-tests each on a real Apple Silicon Mac, Intel
Mac and Windows machine, and publishes the zips as a GitHub release. Creating
the release on GitHub instead (Releases → *Draft a new release* with a new
`v*` tag) works too: the zips are attached to it when the run finishes, and
the standard install notes are added if you left the description empty. The
builds are not code-signed or notarized, hence the one-time "Open Anyway" /
"Run anyway" steps above; signing needs an Apple Developer ID and a Windows
code-signing certificate.
