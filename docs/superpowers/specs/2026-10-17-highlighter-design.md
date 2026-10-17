# highlighter — Design Spec

**Date:** 2026-10-17
**Status:** Approved (grilling session, Q1–Q24)

## Purpose

Turn a public YouTube video of a basketball fun-game match into one MP4 per play.
The user pastes the video URL, enters the start timestamp of each play, reviews
each clip's duration (default 10 s) in an embedded player, and then processes.
Only the needed sections are downloaded and every clip keeps the source aspect ratio.

Single user, running locally on macOS (Apple Silicon).

## Architecture

```
┌──────────── browser (localhost:7777) ────────────┐
│ Vite SPA: React + TanStack Router + TanStack     │
│ Query + Tailwind + shadcn/ui                     │
│ YouTube IFrame Player (review: no download)      │
└───────────────┬──────────────────────────────────┘
                │ JSON REST + Server-Sent Events
┌───────────────▼──────────────────────────────────┐
│ highlighter (single Go binary, UI embedded)      │
│  timecode · clip · store · ytdlp · ffmpeg · job  │
│  server · tools                                  │
└───────┬──────────────────────────┬───────────────┘
        │ exec                      │ exec
   yt-dlp -J (stream URLs)     ffmpeg -ss … -i <URL> (section fetch + encode)
```

- **Go** backend. yt-dlp and ffmpeg run as subprocesses. No libav bindings.
- yt-dlp and ffmpeg come from **Homebrew** (`brew install yt-dlp ffmpeg`). The app
  checks for them at startup and on `/api/health`. The UI has an "Update yt-dlp"
  button that runs `brew upgrade yt-dlp`.
- The server listens on `127.0.0.1:7777` only and opens the browser on start
  (`-no-open` disables this, `-port` changes the port).
- Config and state live in `~/.config/highlighter/`: `settings.json` plus
  `sessions/<videoId>.json`.

## Download and encode pipeline

1. `yt-dlp -J --no-playlist -f "bv*[height<=1080]+ba/b[height<=1080]/b" -S vcodec:h264 <url>`
   resolves the title, the duration, and the direct video and audio stream URLs
   (plus the HTTP headers they need).
2. For each clip, one ffmpeg process does input-seeking (`-ss` before `-i`)
   on the stream URLs. It fetches only the bytes it needs and re-encodes
   frame-accurately:
   - Video: `h264_videotoolbox` (8 Mb/s), falling back to `libx264 -preset veryfast -crf 20`.
     The encoder is chosen at startup with a tiny test encode.
   - Audio: AAC 160 kb/s. MP4 with `+faststart`. `yuv420p`.
   - No scaling or cropping, so the source resolution and aspect ratio are kept.
     Quality is capped at 1080p by the format selector.
3. Download and encode happen in **one ffmpeg pass**, so a clip reports a single
   `processing` percentage (from `-progress pipe:1`) rather than separate
   download and encode phases.
4. Up to **3** clips run at once (Settings: 1–8).
5. A failed clip is retried automatically **twice** (2 s and then 5 s backoff), and
   the stream URLs are re-resolved on every retry. If it still fails it is marked failed,
   and a manual Retry button re-runs that clip only. Other clips keep going.
6. Output is written to `<file>.part` and renamed when complete.
7. **Output mode** (chosen per run): *every clip* (default), *full reel only*, or *both*.
   The reel is built with the ffmpeg concat demuxer (`-c copy`) once every clip is done.
   Its name defaults to the video title lowercased with spaces → `_` (e.g. `me_at_the_zoo.mp4`)
   and can be renamed; typed names are normalised the same way. In reel-only mode the clips are
   cut into `<outputDir>/.parts/` and that folder is removed once the reel is built (kept on failure
   so a retry can reuse it). Finished clips and the reel report their length and file size. If clips
   failed, it waits for their manual retries. If the reel itself fails, it can be retried.

## Domain rules

- **Timestamp formats:** `83`, `1:23`, `01:02:03`, optional fraction (`1:23.5`).
  Minutes and seconds after the first field must be < 60.
- **Paste list:** one entry per line, `<timestamp> [label]`. Separators like
  `-`, `–`, `|`, `:` or `,` between the timestamp and label are stripped, including one glued to the
  timestamp (`3:47: 3PT` → 3:47, label `3PT`). A line made
  up only of timestamps separated by commas, semicolons or spaces becomes several
  entries. Invalid lines are reported with their line number and stay in the textarea.
  CRLF is accepted.
- **Clip:** start, duration (default 10 s, min 1 s, max 120 s), optional label, approved flag.
  Clips are always kept sorted by start time and numbered 1..N in that order.
- **Lead-in:** global, 0–30 s, default 0. It *extends* every clip backwards:
  effective range = `[max(0, start − leadIn), start + duration]`. The end of the play
  does not move.
- **Video end:** a clip that *starts* at or after the video's end is an **error**.
  A clip that *ends* after the video's end is a **warning** and is trimmed to the end.
- **Warnings** (do not block processing): overlap with an earlier clip, same
  start as an earlier clip (within 0.05 s), trimmed at video end.
- **Errors** (block processing): duration outside 1–120 s, negative start, start
  at or after the video end.
- **Strict approval:** Process is enabled only when there is ≥ 1 clip, every clip
  is approved, and there are no errors. Changing a clip's start or duration, or
  changing the lead-in, clears approval (for that clip, or for all clips in the case of lead-in).
  Label edits keep approval. "Approve all remaining" exists.
- **Output folder:** `<outputRoot>/<safe video title>/`. The default outputRoot is
  `~/Movies/Highlights`. Unsafe characters `/\:*?"<>|` and control chars become `-`,
  the name is capped at 80 characters, and an empty title becomes `untitled`.
- **File name:** `NN_<m-ss or h-mm-ss>[_label-slug].mp4`, e.g.
  `03_12-34_andi-crossover.mp4`. The slug is lowercase, letters and digits only, runs of other characters become `-`, ≤ 40 chars.
- **Accepted URLs:** `youtube.com/watch?v=`, `youtu.be/`, `/shorts/`, `/live/`,
  `/embed/` on `www.`, `m.` or `music.` hosts, with or without a scheme. Extra query parameters
  (`t`, `list`, `si`) are ignored. The canonical form is `https://www.youtube.com/watch?v=<id>`.
  Live and upcoming streams are rejected.

## UI

- **Home** (`/`): tool status banner (missing tools → `brew install yt-dlp ffmpeg`,
  "Update yt-dlp" button), URL input → opens/creates the session, "Recent videos" list,
  Settings card (output root, concurrency).
- **Session** (`/s/$videoId`):
  - Embedded YouTube player with a live time readout and an **Add clip at m:ss** button (adds a clip at the current time, rounded down to the second).
  - Paste textarea → "Add clips".
  - Lead-in input.
  - Clip list: index, start ±1 s, label, duration with −2 / +2 / +5 buttons and a
    number field, ▶ Preview (seeks to the effective start, auto-pauses at the effective end),
    "Looks good" toggle, Remove, and the clip's warnings and errors. Previewing the same clip again
    resumes from the playhead while it is inside the clip; changing the duration mid-preview moves the
    end point without restarting. The clip being previewed shows a countdown of the seconds left.
  - "Approve all remaining".
  - Process panel: output mode (every clip / full reel / both), reel file name, Process button
    (with the reason when it is disabled), and per-clip live status (queued → processing % →
    done/failed) via SSE. Done clips get an inline `<video>` and a download link, failed clips get
    the error and Retry. "Open folder".
- Controls are disabled while a save is in flight so that rapid clicks can't lose updates.

## API (all JSON; errors are `{"error": "..."}`)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/health` | yt-dlp/ffmpeg found + version, active encoder |
| POST | `/api/tools/update-ytdlp` | `brew upgrade yt-dlp`, returns output |
| GET/PUT | `/api/settings` | `{outputRoot, concurrency}` |
| GET | `/api/sessions` | recent sessions |
| POST | `/api/sessions` | `{url}` → load existing session or probe and create it |
| GET/PUT | `/api/sessions/{id}` | view / save `{leadIn, clips}` |
| POST | `/api/sessions/{id}/clips/parse` | `{text}` → append clips, returns `{session, errors}` |
| POST | `/api/sessions/{id}/process` | `{mode, reelName}` → 202 job view; 400 bad mode or reel name clashing with a clip; 409 if not processable or busy |
| POST | `/api/sessions/{id}/retry/{clipId}` | retry a failed clip (`concat` = the reel) |
| GET | `/api/sessions/{id}/events` | SSE: full job view on every change |
| POST | `/api/sessions/{id}/open-folder` | `open <outputDir>` |
| GET | `/api/sessions/{id}/files/{name}` | serve a finished `.mp4` |

Every other path serves the embedded SPA, with an `index.html` fallback.

## Repo layout

```
cmd/highlighter/main.go
internal/{timecode,clip,store,ytdlp,ffmpeg,tools,job,server,ui}/
e2e/                      (build tag e2e, needs network + tools)
web/                      (Vite app; builds into internal/ui/dist)
Makefile                  (build, test, e2e, dev-api, dev-web)
```

## Testing

- TDD unit tests: timecode parsing, clip math, validation, reconciliation and naming,
  store, URL/info parsing, ffmpeg argument building and progress, job manager (fakes),
  HTTP API (httptest + fakes).
- `make e2e`: clips 3 s of `https://www.youtube.com/watch?v=jNQXAC9IVRw` with the real
  tools, then asserts duration ≈ 3 s and the source dimensions are preserved.
- UI: manual checklist only in v1.

## Out of scope (v1)

Vertical or cropped Shorts output, overlays and watermarks, automatic play detection,
uploading to YouTube, private or members-only videos, live streams, multi-user or hosted deployment.
