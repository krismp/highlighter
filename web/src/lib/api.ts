export type Severity = "error" | "warning";
export interface Issue { clipId: string; severity: Severity; message: string }
export interface Clip { id: string; start: number; duration: number; label: string; approved: boolean }
export interface ClipView extends Clip { index: number; startText: string; effStart: number; effDuration: number; file: string }
export interface SessionView {
  videoId: string; url: string; title: string; duration: number; durationText: string; leadIn: number;
  clips: ClipView[]; issues: Issue[]; canProcess: boolean; outputDir: string; reelName: string;
  /** The page opens before yt-dlp has fetched the length; these say how that is going. */
  resolving: boolean; resolveError: string;
}
export type ClipInput = Omit<Clip, "id"> & { id?: string };
export interface RecentSession { videoId: string; title: string; clipCount: number; updatedAt: string }
export interface ToolStatus { found: boolean; path: string; version: string }
export interface Health { ytDlp: ToolStatus; ffmpeg: ToolStatus; deno: ToolStatus; encoder: string; os: string; version: string }
export interface Connectivity { online: boolean; error: string }
export interface Settings { outputRoot: string; concurrency: number; defaultDuration: number }
export interface LineError { line: number; text: string; err: string }
export type JobStatus = "queued" | "processing" | "done" | "failed";
export interface ClipState { clipId: string; file: string; status: JobStatus; progress: number; attempt: number; error: string; duration: number; size: number; trashed: boolean }
/** Which files a run leaves in the folder: each clip, only the combined reel, or both. */
export type OutputMode = "clips" | "reel" | "both";
export interface JobView {
  sessionId: string; outputDir: string; mode: OutputMode; clips: ClipState[]; running: boolean;
  concat: { requested: boolean; status: JobStatus | ""; file: string; error: string; duration: number; size: number; trashed: boolean };
}

export interface Media {
  duration: number; width: number; height: number; rotation: number; fps: number; videoCodec: string; pixFmt: string;
  hasAudio: boolean; audioCodec: string; sampleRate: number; channels: string; size: number;
}
export interface CombineItem { id: string; path: string; name: string; folder: string; media: Media; missing: boolean }
/** The combined video's frame: the first video's, or a standard size. */
export type CombineSize = "auto" | "1080p" | "720p" | "vertical" | "square";
export type Transition = "cut" | "fade";
export interface CombineJob {
  status: "" | "processing" | "done" | "failed" | "canceled"; running: boolean; progress: number; copy: boolean; count: number;
  dir: string; file: string; width: number; height: number; duration: number; size: number; error: string; startedAt: string; trashed: boolean;
}
export interface CombineView {
  items: CombineItem[]; name: string; defaultName: string; size: CombineSize; transition: Transition; outputDir: string;
  total: number; width: number; height: number; copy: boolean; job: CombineJob;
}
export interface CombineChoices { name: string; size: CombineSize; transition: Transition }
export interface AddError { path: string; name: string; error: string }
export interface LibraryFile { path: string; name: string; folder: string; size: number; modTime: string }

export class ApiError extends Error {
  status: number;  // 0 when the server could not be reached at all
  request: string; // "POST /api/sessions → 500", for error reports
  constructor(status: number, message: string, request: string) {
    super(message);
    this.status = status;
    this.request = request;
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const where = `${method} /api${path}`;
  let res: Response;
  try {
    res = await fetch(`/api${path}`, {
      method,
      headers: method === "GET" ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (err) {
    throw new ApiError(0, "Can't reach highlighter's local server. If the app was closed or crashed, open it again.",
      `${where} → no response (${err instanceof Error ? err.message : String(err)})`);
  }
  const request = `${where} → ${res.status} ${res.statusText}`;
  const text = await res.text();
  let data: unknown;
  try {
    data = text ? JSON.parse(text) : undefined;
  } catch {
    throw new ApiError(res.status, `The server sent an unexpected response (HTTP ${res.status}).`, `${request}\n${text.slice(0, 500)}`);
  }
  if (!res.ok) {
    const message = (data as { error?: string } | undefined)?.error ?? `Request failed (HTTP ${res.status} ${res.statusText}).`;
    throw new ApiError(res.status, message, request);
  }
  return data as T;
}

export const toInputs = (clips: ClipView[]): ClipInput[] =>
  clips.map(({ id, start, duration, label, approved }) => ({ id, start, duration, label, approved }));

export const api = {
  health: () => request<Health>("GET", "/health"),
  connectivity: () => request<Connectivity>("GET", "/connectivity"),
  updateYtDlp: () => request<{ output: string }>("POST", "/tools/update-ytdlp"),
  /** Videos still being processed (and a running combine), which an app restart would stop. */
  runningJobs: () => request<{ running: { sessionId: string; title: string }[] }>("GET", "/jobs/running"),
  quit: () => request<void>("POST", "/quit"),
  settings: () => request<Settings>("GET", "/settings"),
  saveSettings: (s: Settings) => request<Settings>("PUT", "/settings", s),
  recent: () => request<RecentSession[]>("GET", "/sessions"),
  openVideo: (url: string) => request<SessionView>("POST", "/sessions", { url }),
  session: (id: string) => request<SessionView>("GET", `/sessions/${id}`),
  /** Fetches the video's details again after a failure. */
  retryResolve: (id: string) => request<SessionView>("POST", `/sessions/${id}/resolve`, {}),
  saveSession: (id: string, body: { leadIn: number; clips: ClipInput[] }) =>
    request<SessionView>("PUT", `/sessions/${id}`, body),
  parseClips: (id: string, text: string) =>
    request<{ session: SessionView; errors: LineError[] }>("POST", `/sessions/${id}/clips/parse`, { text }),
  process: (id: string, mode: OutputMode, reelName: string) =>
    request<JobView>("POST", `/sessions/${id}/process`, { mode, reelName }),
  retry: (id: string, clipId: string) => request<JobView>("POST", `/sessions/${id}/retry/${clipId}`),
  openFolder: (id: string) => request<void>("POST", `/sessions/${id}/open-folder`),
  /** Moves a finished file to the Trash; returns the updated results when the file belongs to the last run. */
  trash: (id: string, file: string) => request<JobView | undefined>("POST", `/sessions/${id}/files/${encodeURIComponent(file)}/trash`),
  deleteSession: (id: string) => request<void>("DELETE", `/sessions/${id}`),
  reveal: (id: string, file: string) => request<void>("POST", `/sessions/${id}/files/${encodeURIComponent(file)}/reveal`),
  fileUrl: (id: string, file: string) => `/api/sessions/${id}/files/${encodeURIComponent(file)}`,
  eventsUrl: (id: string) => `/api/sessions/${id}/events`,

  combine: () => request<CombineView>("GET", "/combine"),
  /** Keeps the videos in itemIds, in that order, and saves the choices. */
  saveCombine: (itemIds: string[], c: CombineChoices) => request<CombineView>("PUT", "/combine", { itemIds, ...c }),
  addVideos: (paths: string[]) => request<{ combine: CombineView; errors: AddError[] }>("POST", "/combine/items", { paths }),
  clearCombine: () => request<CombineView>("DELETE", "/combine"),
  startCombine: () => request<CombineJob>("POST", "/combine/start", {}),
  /** Forgets the last result so the line-up can be changed; the file stays. */
  dismissCombined: () => request<CombineView>("POST", "/combine/dismiss", {}),
  cancelCombine: () => request<void>("POST", "/combine/cancel", {}),
  library: () => request<{ root: string; files: LibraryFile[] }>("GET", "/combine/library"),
  revealCombined: () => request<void>("POST", "/combine/result/reveal", {}),
  trashCombined: () => request<CombineJob>("POST", "/combine/result/trash", {}),
  openCombinedFolder: () => request<void>("POST", "/combine/open-folder", {}),
  itemThumbUrl: (id: string) => `/api/combine/items/${id}/thumb`,
  itemFileUrl: (id: string) => `/api/combine/items/${id}/file`,
  libraryThumbUrl: (path: string) => `/api/combine/library/thumb?path=${encodeURIComponent(path)}`,
  combinedUrl: (file: string) => `/api/combine/result?f=${encodeURIComponent(file)}`,
  combineEventsUrl: "/api/combine/events",
};
