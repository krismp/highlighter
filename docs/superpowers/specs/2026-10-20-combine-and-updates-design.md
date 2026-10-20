# Combine videos and in-app updates — Design

**Date:** 2026-10-20

## 1. Combine videos

Join any videos on the computer (finished highlight clips from different
matches, phone videos, a whole match) into one MP4, in an order the user
chooses.

### Page: `/combine` ("Combine" in the header, a card on the home page)

- **Empty state:** a large drop zone, "Drop videos here", with an **Add videos…**
  button (native multi-file dialog) and **From your highlights**: the newest
  MP4s in the output folder, grouped by match, one click to add.
- **Line-up:** the filmstrip on top shows every video as a segment whose width
  is its share of the total length; below it, one row per video with a
  thumbnail (click to preview), name, length, resolution and a "no sound" tag.
  Rows are reordered by dragging the handle (or with the keyboard: focus the
  handle, Space, arrows) and removed with ×. Adding more is always one click
  or one drop away.
- **Output panel:** file name, **Size** (Match first video · 1080p · 720p ·
  Vertical 1080×1920 · Square 1080×1080), **Between videos** (Cut · Fade
  through black), a summary (N videos · total length · saved to …) and the
  big **Combine N videos** button.
- **Running:** striped progress bar with percentage and time left, Cancel.
- **Done:** the result plays inline; Show in Finder/File Explorer, Open
  folder, Start a new combination (clears the list; the files stay).
- Dropping video files anywhere in the app opens this page with them added.

### Server

- The line-up is stored in `~/.config/highlighter/combine.json`, so it
  survives reloads and restarts. Each item stores its path and what probing
  found (`ffmpeg -i`: duration, size, rotation, fps, codecs, audio).
- Files are only ever served (thumbnail, preview) by item id, never by path,
  and items are only added with JSON POSTs, which other sites cannot send.
- `combine.Plan` picks the render: when every video has identical H.264/AAC
  parameters, no fades and "Match first video", it joins with stream copy
  (seconds, lossless). Otherwise one ffmpeg pass scales and pads every input to
  the target size (keeping aspect ratio, black bars), equalises frame rate and
  audio (silent track for videos without sound), optionally fades, and
  concatenates. Progress comes from `-progress`.
- Output: `<output folder>/Combined/<name>.mp4`, written via `.part` and never
  overwriting: an existing name gets " 2", " 3"…
- One combine runs at a time; it counts as running work for the quit warning
  and is stopped on shutdown.

### Desktop shell

The UI is served by the local server, so it is a *remote* origin to Tauri. A
capability scoped to `http://127.0.0.1:*` and `http://localhost:*` grants only:
the dialog plugin's `open`, event listening (for file drops) and the app's
update commands. In a plain browser (development) the page falls back to the
highlights list and shows how to add files.

## 2. Updates without reinstalling

Yes, it's possible: Tauri's updater replaces the app in place.

- **macOS:** downloads `Highlighter.app.tar.gz`, verifies its signature,
  swaps the bundle in /Applications, restarts. No new "Open Anyway" prompt,
  because the app downloads the file itself, so it isn't quarantined.
- **Windows:** downloads the NSIS setup, verifies it, runs it in passive mode
  over the existing install (settings and clips untouched) and restarts.
- The update is signed with a Tauri updater key (separate from code
  signing). The private key lives in the `TAURI_SIGNING_PRIVATE_KEY` GitHub
  secret; the public key is in `tauri.conf.json`.
- CI builds the updater bundles next to the zips and the release job writes
  `latest.json`, served from
  `https://github.com/krismp/highlighter/releases/latest/download/latest.json`.
- **UI:** the app checks quietly once at launch; when an update exists, a slim
  bar under the header says "Highlighter X is available" with **Update now**
  and **Later** (Later hides it for that version). Settings → **Updates** shows
  the current version, **Check for updates**, the release notes, and a
  progress bar while downloading. If videos are still being processed, it
  asks before restarting.
- Before installing, the shell stops the server (and its ffmpeg/yt-dlp runs),
  then installs and restarts.
