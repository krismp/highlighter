# Videos Menu, Drafts and Unfinished Badge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A header "Videos" menu on every page listing recent videos with a status, a badge counting unfinished ones, and paste-box text that survives leaving the page.

**Architecture:** The server stores three new session fields (`draft`, `changedAt`, `processedAt`) and derives a status per video in `GET /api/sessions`. The job manager calls a hook when a job stops so the server can stamp `processedAt`. The web app adds a `VideosMenu` popover in the header, a shared `StatusChip`, and debounced draft saving in `AddClips`.

**Tech Stack:** Go 1.x stdlib HTTP server; React 19 + TanStack Query/Router, Tailwind, `@base-ui/react` (Popover), lucide-react.

**Spec:** `docs/superpowers/specs/2026-11-21-videos-menu-drafts-design.md`

## Global Constraints

- Statuses are exactly `processing`, `draft`, `done`, `empty`; only `processing` and `draft` count in the badge.
- `draft` = clips changed since the last successful run, or non-empty paste text. `done` = last run succeeded and nothing changed since.
- Old sessions (zero `changedAt` and zero `processedAt`) are `done` if every clip's file or the reel file exists in the output folder, else `draft` (if they have clips).
- A run is successful only when every clip is `done` and, if a reel was requested, the reel is `done`.
- Popover lists at most 8 videos, unfinished first, then by `updatedAt` desc; footer link "See all videos" to Home's Recent list.
- Badge uses the court-orange colour (`bg-court`); spinner (`LoaderCircle animate-spin`) while any video is processing.
- No layout change beyond the header item; no new npm dependencies.

## Review Focus

1. Toggling **Looks good / Approved** on a finished video must not turn it back into a draft. → test in Task 2.
2. A run where any clip (or the requested reel) fails must leave the video a draft. → test in Task 2.
3. A pending debounced draft save must not overwrite the text left after **Add clips** (the failed lines). → Task 5 cancels the pending save; manual check in Task 5.
4. An old video processed as "reel only" (parts folder deleted) must show as done. → test in Task 1.
5. A video deleted from Recent while its job runs must not crash the server when the job finishes. → test in Task 2.

---

### Task 1: Session fields and status in the recent list

**Files:**
- Modify: `internal/store/store.go:27-41` (Session struct)
- Create: `internal/server/status.go`
- Create: `internal/server/status_test.go`
- Modify: `internal/server/view.go:37-42` (RecentSession)
- Modify: `internal/server/server.go:311-322` (listSessions)

**Interfaces:**
- Produces: `store.Session` fields `Draft string \`json:"draft,omitempty"\``, `ChangedAt time.Time \`json:"changedAt,omitzero"\``, `ProcessedAt time.Time \`json:"processedAt,omitzero"\``.
- Produces: `type Status string` with consts `StatusProcessing = "processing"`, `StatusDraft = "draft"`, `StatusDone = "done"`, `StatusEmpty = "empty"`.
- Produces: `func videoStatus(sess store.Session, running bool, outputDir string) Status` in `status.go`.
- Produces: `RecentSession.Status Status \`json:"status"\``.

- [ ] **Step 1: Write the failing tests** in `status_test.go`:

```go
func TestVideoStatus(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Date(2026, 11, 21, 8, 0, 0, 0, time.UTC)
	withClips := store.Session{Title: "Game 1", Clips: []clip.Clip{{ID: "a", Start: 5, Duration: 10}}}
	cases := []struct {
		name    string
		sess    store.Session
		running bool
		want    Status
	}{
		{"running wins", withClips, true, StatusProcessing},
		{"no clips, no text", store.Session{}, false, StatusEmpty},
		{"only paste text", store.Session{Draft: "1:23"}, false, StatusDraft},
		{"changed after run", with(withClips, t0.Add(time.Minute), t0), false, StatusDraft},
		{"run after change", with(withClips, t0, t0.Add(time.Minute)), false, StatusDone},
		{"done but text left", withDraft(with(withClips, t0, t0.Add(time.Minute)), "2:00"), false, StatusDraft},
		{"old, no files", withClips, false, StatusDraft},
	}
	for _, c := range cases {
		if got := videoStatus(c.sess, c.running, dir); got != c.want {
			t.Errorf("%s: %q; want %q", c.name, got, c.want)
		}
	}
}

// Old sessions have no timestamps: their files on disk decide.
func TestVideoStatusOldSessionFiles(t *testing.T) {
	sess := store.Session{Title: "Game 1", Clips: []clip.Clip{{ID: "a", Start: 5, Duration: 10}, {ID: "b", Start: 30, Duration: 10}}}
	clipsDir, reelDir := t.TempDir(), t.TempDir()
	for i, c := range sess.Clips {
		os.WriteFile(filepath.Join(clipsDir, clip.Filename(i+1, c)), nil, 0o644)
	}
	os.WriteFile(filepath.Join(reelDir, clip.ReelName(sess.Title)+".mp4"), nil, 0o644) // "reel only": no clip files
	if got := videoStatus(sess, false, clipsDir); got != StatusDone {
		t.Errorf("every clip file: %q; want done", got)
	}
	if got := videoStatus(sess, false, reelDir); got != StatusDone {
		t.Errorf("reel file: %q; want done", got)
	}
}
```

`with(s, changedAt, processedAt)` and `withDraft(s, text)` are small test helpers returning a modified copy.

- [ ] **Step 2: Run, expect FAIL** — `go test ./internal/server -run VideoStatus` → build failure (undefined `videoStatus`).

- [ ] **Step 3: Implement** the store fields, `Status`, and `videoStatus` in `status.go`. Order of checks: running → processing; `Draft` non-empty (after TrimSpace) → draft; no clips → empty; both timestamps zero → files check (all `clip.Filename(i+1, c)` exist in `outputDir`, or `clip.ReelName(sess.Title)+".mp4"` exists) → done else draft; `!ProcessedAt.IsZero() && !ProcessedAt.Before(ChangedAt)` → done; else draft.

- [ ] **Step 4: Wire into `listSessions`**: load settings via `s.settings(w)`; for each session `Status: videoStatus(sess, jobRunning(s.Jobs, sess.VideoID), outputDir(st.OutputRoot, sess.Title))`, where running comes from `s.Jobs.View(id)` (`ok && v.Running`). Add a server test `TestRecentListsStatus`: open a video, add clips → recent entry status `draft`.

- [ ] **Step 5: Run** `go test ./internal/...` → PASS.

- [ ] **Step 6: Commit** — `feat(server): video status in the recent list`.

---

### Task 2: Track changes and successful runs

**Files:**
- Modify: `internal/job/job.go` (Manager field + calls in `runClip`, `runConcat`)
- Modify: `internal/job/job_test.go`
- Modify: `internal/server/server.go` (`New`, `putSession`, `parseClips`, new `markProcessed`)
- Modify: `internal/server/server_test.go`

**Interfaces:**
- Consumes: `store.Session.ChangedAt`, `ProcessedAt` (Task 1).
- Produces: `Manager.OnFinish func(View)` — called once each time a job goes from running to not running (after the final `update`), outside any lock. `nil` = no call.
- Produces: `func (v View) Succeeded() bool` in `job.go` — every clip `Done` and (`!v.Concat.Requested || v.Concat.Status == Done`).
- Produces: `func timingChanged(old, new []clip.Clip, oldLeadIn, newLeadIn float64) bool` in `server.go` — true if lead-in, count, or any clip's ID/Start/Duration/Label differs (index by index); `Approved` is ignored.

- [ ] **Step 1: Write failing job tests**: `TestOnFinishCalledOnceWhenJobStops` (2 clips, reel mode; hook called exactly once, with `Succeeded() == true`); `TestOnFinishReportsFailure` (cutter fails one clip on every attempt; hook called once with `Succeeded() == false`); after a manual `Retry` that succeeds, hook called again with `Succeeded() == true`.

- [ ] **Step 2: Write failing server tests**:
  - `TestProcessMarksDone`: add clips, approve all via PUT, process, wait for job end → recent status `done`.
  - `TestApproveAfterDoneStaysDone`: after the above, PUT the same clips with `approved` flipped → still `done`.
  - `TestEditAfterDoneIsDraft`: after done, PUT with one clip's duration +1 → `draft`.
  - `TestFailedRunStaysDraft`: use `newEnvWith` and a cutter that always fails → after job ends, `draft`.
  - `TestDeletedWhileProcessing`: start a job with a slow cutter, `DELETE /api/sessions/{id}`, let it finish → no panic, `GET /api/sessions` doesn't list it.

- [ ] **Step 3: Run, expect FAIL** — `go test ./internal/job ./internal/server`.

- [ ] **Step 4: Implement**: `OnFinish` and `Succeeded` in job; in `runClip` decide "just stopped" inside the final `update` closure (`!j.viewLocked().Running`) and call the hook after it (when concat starts, `runConcat` is the one that calls it). In server `New`, set `d.Jobs.OnFinish = s.markProcessed` when `d.Jobs != nil`. `markProcessed(v job.View)`: if `!v.Succeeded()` return; take `editMu`; `LoadSession(v.SessionID)` — on error return silently; set `ProcessedAt = s.Store` clock now (use `time.Now().UTC()`); save. In `putSession` and `parseClips` set `sess.ChangedAt = time.Now().UTC()` when `timingChanged(...)` (parseClips: when it added any entry).

- [ ] **Step 5: Run** `go test ./...` → PASS.

- [ ] **Step 6: Commit** — `feat: remember when a video was last processed`.

---

### Task 3: Draft text endpoint

**Files:**
- Modify: `internal/server/server.go` (route + `putDraft`, `parseClips`)
- Modify: `internal/server/view.go` (SessionView `Draft`)
- Modify: `internal/server/server_test.go`

**Interfaces:**
- Produces: `PUT /api/sessions/{id}/draft` with body `{"text": string}` → `204`. Saves `sess.Draft`; does not touch `ChangedAt`.
- Produces: `SessionView.Draft string \`json:"draft"\``.
- Behaviour: `parseClips` sets `sess.Draft` to the failed lines' text joined by `\n` (empty when all lines parsed).

- [ ] **Step 1: Write failing tests**: `TestDraftSavedAndShown` (PUT draft "PLAY: 1:00 - 1:10" → GET session has that `draft`; recent status `draft`); `TestParseKeepsFailedLinesAsDraft` (parse "0:05\nbanana" → session `draft == "banana"`); draft over 64 KB → 400.

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement** `putDraft` (under `editMu`, `readJSON`, length check 64<<10, `SaveSession`, `w.WriteHeader(204)`), route, view field, parseClips change.

- [ ] **Step 4: Run** `go test ./...` → PASS.

- [ ] **Step 5: Commit** — `feat(server): save the paste box as a draft`.

---

### Task 4: Status chip and Home's Recent list

**Files:**
- Modify: `web/src/lib/api.ts`
- Create: `web/src/components/StatusChip.tsx`
- Create: `web/src/hooks/useRecent.ts`
- Modify: `web/src/routes/HomePage.tsx`

**Interfaces:**
- Produces (api.ts): `export type VideoStatus = "processing" | "draft" | "done" | "empty"`; `RecentSession.status: VideoStatus`; `SessionView.draft: string`; `api.saveDraft(id: string, text: string): Promise<void>` (PUT `/sessions/${id}/draft`).
- Produces: `export const unfinished = (s: VideoStatus) => s === "processing" || s === "draft"` in `api.ts`.
- Produces: `useRecent()` — the `["recent"]` query (`api.recent`), `refetchInterval` 2000 ms while any entry is `processing`, otherwise off.
- Produces: `<StatusChip status={VideoStatus} />` — `processing`: spinner + "processing"; `draft`: court-orange dot + "draft"; `done`: `Check` icon in the net colour + "done"; `empty`: renders nothing.

- [ ] **Step 1: Implement** the api additions, `useRecent`, `StatusChip`.
- [ ] **Step 2: Home**: use `useRecent()`; give the Recent `<section>` `id="recent"`; show `<StatusChip>` before the clip count in each row.
- [ ] **Step 3: Verify** — `cd web && pnpm build` succeeds; `pnpm lint` shows no new warnings in touched files.
- [ ] **Step 4: Commit** — `feat(web): status on recent videos`.

---

### Task 5: Videos menu in the header and draft-saving paste box

**Files:**
- Create: `web/src/components/VideosMenu.tsx`
- Modify: `web/src/router.tsx:20` (insert before the Combine link)
- Modify: `web/src/components/AddClips.tsx`
- Modify: `web/src/routes/SessionPage.tsx` (pass `draft` to AddClips; invalidate `["recent"]` after saves)
- Modify: `README.md` ("Good to know": one bullet on the Videos menu and drafts)

**Interfaces:**
- Consumes: `useRecent`, `StatusChip`, `unfinished`, `api.saveDraft`, `SessionView.draft` (Task 4).
- Produces: `<VideosMenu />`; `AddClips` props become `{ videoId, initialDraft: string, onAdded }`.

- [ ] **Step 1: VideosMenu**: `@base-ui/react` Popover. Trigger styled like the Combine link, `Clapperboard` icon + "Videos"; badge = count of `unfinished` entries (hidden at 0), spinner instead when any is `processing`. Panel: up to 8 entries sorted unfinished-first then `updatedAt` desc; each row a `Link` to `/s/$videoId` (title truncated, `N clips`, `StatusChip`), closing the popover on click; footer `Link to="/" hash="recent"` "See all videos". Empty list: "No videos yet. Paste a YouTube link on Home."
- [ ] **Step 2: AddClips draft**: initial `text` = `initialDraft`; on change schedule `api.saveDraft` 500 ms later (ref-held timer, reset each keystroke), then invalidate `["recent"]`. On **Add clips**, clear the pending timer before mutating. Flush (send immediately) on unmount if a save is pending.
- [ ] **Step 3: SessionPage**: `<AddClips initialDraft={session.data.draft} ...>`; in `save` and parse success handlers also `qc.invalidateQueries({ queryKey: ["recent"] })`; same after starting processing in `ProcessPanel`'s `onSuccess`.
- [ ] **Step 4: Verify** — `pnpm build`, `pnpm lint` clean for touched files; `go test ./...` still PASS. Manual check in the app: type in the paste box, go to Settings, Videos menu shows ● 1 and the video as draft, open it, text is back; press Add clips with one bad line, leave and return within a second, only the bad line remains.
- [ ] **Step 5: Commit** — `feat(web): Videos menu with unfinished badge; paste box keeps a draft`.
