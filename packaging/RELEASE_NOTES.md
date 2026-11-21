## What's new in 1.3.1

- Release notes in **Settings → Updates** are now properly formatted, with headings, lists and bold text, instead of raw Markdown. The download and install steps are no longer shown there.

## What's new in 1.3.0

### Get back to any video from anywhere
A new **Videos** button sits in the top bar on every page. Click it to see your recent videos and jump straight back into one, even from Combine or Settings.

- An orange badge on the button shows how many videos are still unfinished. While a video is being processed, the badge shows a spinner.
- Each video shows its status: **draft** (still needs work), **processing**, or **done**. Recent videos on the Home page show the same status.
- A video counts as done once it has been processed and nothing changed since. Editing a clip's start, length or label afterwards makes it a draft again; ticking **Looks good** doesn't.

### Your pasted times are kept
Text in the paste box is now saved as you type. If you leave the page before clicking **Add clips**, it's still there when you come back.

### Smaller improvements
- **Full reel** and **Both** now show how long the joined video will be.
- The **Length** box fits longer clips like `210` seconds, and shows the time in minutes (`3:30`) once a clip is over a minute.

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
