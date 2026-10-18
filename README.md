# highlighter

Paste a YouTube URL, mark the start of each play, review each clip in the
embedded player, and get one MP4 per play. The source aspect ratio is kept.

## Install

Download the file for your computer from the
[latest release](https://github.com/krismp/highlighter/releases/latest):

| Your computer | Download |
| --- | --- |
| Mac with Apple Silicon (M1 and later) | `Highlighter-<version>-macos-apple-silicon.dmg` |
| Mac with an Intel processor | `Highlighter-<version>-macos-intel.dmg` |
| Windows 10 or 11, 64-bit | `Highlighter-<version>-windows-x64.zip` |

Not sure which Mac you have? Apple menu → **About This Mac**: "Chip: Apple M…"
is Apple Silicon, "Processor: … Intel" is Intel.

Each download includes everything highlighter needs (yt-dlp, ffmpeg and
deno), so there is nothing else to install. It needs macOS 12 or later, or
Windows 10 or later.

### macOS

1. Double-click the `.dmg`, then drag **Highlighter** onto the **Applications** folder.
2. Open **Applications** and double-click **Highlighter**.
3. **First launch only:** the app isn't from the App Store, so macOS says it
   can't verify the developer. Click **Done**, then open **System Settings →
   Privacy & Security**, scroll to *"Highlighter" was blocked…*, click
   **Open Anyway** and confirm with your password or Touch ID.
4. Highlighter opens in your browser at <http://127.0.0.1:7777>. It has no
   window of its own and keeps running in the background:
   - to come back to it, open Highlighter again (it reopens the page);
   - to stop it, click **Quit** at the top right of the page.

Clips are saved to `Movies/Highlights` (changeable in Settings). The log is
at `~/Library/Logs/Highlighter.log`.

If macOS still refuses to open it, run this in Terminal, then open the app again:

```bash
xattr -cr /Applications/Highlighter.app
```

### Windows

1. Right-click the `.zip` → **Extract All…** → **Extract**. (Running it from
   inside the zip does not work.)
2. Move the extracted **Highlighter** folder wherever you like, for example
   *Documents*. Keep its contents together; the `tools` folder is needed.
3. Double-click **Highlighter.exe**.
4. **First launch only:** if Windows shows *"Windows protected your PC"*,
   click **More info → Run anyway**. If the firewall asks, you can click
   Cancel: highlighter only talks to your own computer.
5. Highlighter opens in your browser at <http://127.0.0.1:7777>. A console
   window also opens; keep it open while you work. To stop, click **Quit** at
   the top right of the page, or close the console window.

Tip: right-click `Highlighter.exe` → *Send to* → *Desktop (create shortcut)*.

Clips are saved to `Videos\Highlights` (changeable in Settings).

### Updating yt-dlp

YouTube changes often break older yt-dlp versions. If videos stop loading,
click **Update yt-dlp** on the home page: it downloads the latest yt-dlp into
your profile (`~/.config/highlighter/bin`) and uses it from then on, no new
highlighter download needed.

### Uninstall

Delete `Highlighter.app` (macOS) or the `Highlighter` folder (Windows).
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

## Release

```bash
make dist      # all three installers into dist/ (run on a Mac)
make smoke     # install the Apple Silicon .dmg in a temp folder and clip a real video
```

`scripts/package.sh <target> [version]` builds one installer (`macos-arm64`,
`macos-amd64` or `windows-amd64`). It bundles the latest yt-dlp, deno and
ffmpeg (pinned, checksum-verified) next to the binary; the app looks for its
tools there first, then on PATH.

Pushing a `v*` tag runs `.github/workflows/release.yml`: it builds all three
installers, smoke-tests each on a real Apple Silicon Mac, Intel Mac and
Windows machine, and publishes them as a GitHub release. Creating the release
on GitHub instead (Releases → *Draft a new release* with a new `v*` tag) works
too: the installers are attached to it when the run finishes, and the
standard install notes are added if you left the description empty. The builds are not
code-signed or notarized, hence the one-time "Open Anyway" / "Run anyway"
steps above; signing needs an Apple Developer ID and a Windows code-signing
certificate.
