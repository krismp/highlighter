## What's new in 1.2.0

### Paste exact time ranges
You can now paste a start **and** end time, and Highlighter cuts exactly that part of the video. You no longer need to adjust each clip's length by hand.

```
PLAY + AST: 2:22 - 3:02
PLAY: 3:55 - 4:26
10:45 - 11:15 fast break
```

- The label can go before or after the times: `PLAY: 3:55 - 4:26` and `3:55 - 4:26 PLAY` both work.
- Separate the two times with `-`, `–`, `~` or `to`.
- A single time still works as before and gets your default clip length, for example `12:34 dunk` or `STEAL: 12:34`.
- Numbers inside a label are never mistaken for times. `Player 5 - 1:10` is the play "Player 5" at 1:10.
- If a line can't be read, such as `7:40 - 7:60` or an end time before its start, it stays in the box with an explanation so you can fix it.

### Longer clips
A clip can now be up to **10 minutes** long (previously 2 minutes), so a long possession or a whole sequence fits in one clip. The default length for a single pasted time is still capped at 2 minutes.

## Download

| Your computer | Download |
| --- | --- |
| Mac with Apple Silicon (M1, M2, M3, M4 and later) | `Highlighter-…-macos-apple-silicon.zip` |
| Mac with an Intel processor | `Highlighter-…-macos-intel.zip` |
| Windows 10 or 11 (64-bit) | `Highlighter-…-windows-x64.zip` |

Not sure which Mac you have? Click the Apple menu → **About This Mac**. "Chip: Apple M…" means Apple Silicon; "Processor: … Intel" means Intel.

Highlighter is a desktop app with everything it needs built in (yt-dlp, ffmpeg, deno). Nothing else to install. Each zip contains the installer and an `INSTALL.txt` with these steps.

## Install on macOS (12 Monterey or later)

1. Double-click the downloaded `.zip`, then double-click the `.dmg` inside it.
2. Drag **Highlighter** onto **Applications**.
3. Open Highlighter from your Applications folder. The first time, macOS says it can't verify the developer: click **Done**, open **System Settings → Privacy & Security**, scroll down and click **Open Anyway**, then confirm. You only do this once.

## Install on Windows (10 or 11)

1. Right-click the downloaded `.zip` → **Extract All…** → **Extract**.
2. Double-click `Highlighter-…-windows-x64-setup.exe`. If Windows shows "Windows protected your PC", click **More info → Run anyway**.
3. Follow the installer (no administrator rights needed), then start **Highlighter** from the Start menu.

## Already have Highlighter?

No need to uninstall. If your Highlighter has **Settings → Updates**, it offers this version itself: click **Update and restart**. Otherwise install as above over the old one (on macOS choose **Replace**); your clips and settings stay.

Highlighter opens in its own window; close it (or click **Quit**) to stop. Full instructions and troubleshooting: see the [README](https://github.com/krismp/highlighter#install).
