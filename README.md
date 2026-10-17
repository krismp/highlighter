# highlighter

Paste a YouTube URL, mark the start of each play, review each clip in the
embedded player, and get one MP4 per play. The source aspect ratio is kept.

## Requirements

```bash
brew install go node pnpm yt-dlp ffmpeg
```

## Run

```bash
make build && ./bin/highlighter   # opens http://127.0.0.1:7777
```

Clips are written to `~/Movies/Highlights/<video title>/`, which can be changed in Settings. A re-run after edits can leave older clip files in the folder. Launch the app from a terminal so Homebrew's PATH (yt-dlp, ffmpeg) is available.

## Develop

```bash
make dev-api   # Go API on :7777
make dev-web   # Vite on :5173, proxies /api to :7777
make test      # unit tests
make e2e       # clips a real video (network + yt-dlp + ffmpeg)
```

If downloads start failing, click **Update yt-dlp** on the home page (`brew upgrade yt-dlp`).
