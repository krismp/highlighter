# Videos menu, drafts and unfinished-work badge

## Why

After pasting a YouTube link and timestamps, moving to Combine or Settings
leaves no visible way back. The clips are saved, but the only list of videos
sits at the bottom of Home, reached by clicking the logo. Text in the paste
box that wasn't added yet is lost on navigation. The user wants to get back
from anywhere and to see at a glance which videos still need work.

## Goals

- A **Videos** item in the header, on every page, listing recent videos with
  their status, and a badge counting unfinished ones.
- Text typed in a video's paste box survives leaving the page (a draft).
- Home's Recent videos shows the same status.

Non-goals: a sidebar or layout change; syncing across machines.
Anti-goal: a badge that nags about videos the user already finished. Old
videos processed before this change must not all show as unfinished.

## Approaches

- **A. Videos menu in the header with a badge** (picked). Reachable from every
  page, answers both "get back" and "what's unfinished".
- B. Always-visible sidebar of videos. Easiest switching, but reworks the
  layout and squeezes the YouTube player.
- C. "← Back to <last video>" in the header plus Recent moved up on Home.
  Smallest, but remembers one video and shows no status.

## Design

**Status of a video** (computed by the server, returned in `GET /api/sessions`):

| Status | When | Counts in badge |
| --- | --- | --- |
| `processing` | a job for it is running | yes (badge shows a spinner) |
| `draft` | has clips changed since the last successful run, or unsent paste text | yes |
| `done` | last run succeeded and nothing changed since | no |
| `empty` | no clips and no paste text | no |

**Server**
- Session gains `draft` (paste-box text), `changedAt` (clips or lead-in last
  changed: set by clip edits and parsing) and `processedAt` (set when a job
  ends with every clip, and the reel if requested, done).
- The job manager gets a hook called when a job stops; the server uses it to
  set `processedAt`.
- `PUT /api/sessions/{id}/draft` saves the paste text. Adding clips keeps only
  the lines that failed, as the box does today.
- Old sessions (no `changedAt`/`processedAt`): `done` if every clip's file or
  the reel already exists in the output folder, otherwise `draft`.

**Web**
- Header: `Videos` button left of Combine, with a court-orange count badge
  (or spinner while processing). Opens a popover: up to 8 recent videos with
  title, clip count and status chip, unfinished first; "See all videos" goes
  to Home's Recent list.
- The paste box loads the saved draft and saves as you type (debounced).
- Home's Recent rows get the same status chip.
- The list refreshes when a session is saved and while anything is processing.

```
🏀 highlighter      [Videos ●2]  Combine  Settings  Quit
                    ┌──────────────────────────────────────┐
                    │ ● Thursday Run Game 3   5 clips · draft
                    │ ● Thursday Run Game 2   1 clip · draft
                    │ ✓ Thursday Run Game 1   4 clips · done
                    │   See all videos →
                    └──────────────────────────────────────┘
```

## Left to the builder

Debounce length, popover component, exact chip styling (follow the existing
court/net colours), and test layout.
