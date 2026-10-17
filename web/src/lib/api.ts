export type Severity = "error" | "warning";
export interface Issue { clipId: string; severity: Severity; message: string }
export interface Clip { id: string; start: number; duration: number; label: string; approved: boolean }
export interface ClipView extends Clip { index: number; startText: string; effStart: number; effDuration: number; file: string }
export interface SessionView {
  videoId: string; url: string; title: string; duration: number; durationText: string; leadIn: number;
  clips: ClipView[]; issues: Issue[]; canProcess: boolean; outputDir: string; reelName: string;
}
export type ClipInput = Omit<Clip, "id"> & { id?: string };
export interface RecentSession { videoId: string; title: string; clipCount: number; updatedAt: string }
export interface ToolStatus { found: boolean; path: string; version: string }
export interface Health { ytDlp: ToolStatus; ffmpeg: ToolStatus; deno: ToolStatus; encoder: string; os: string }
export interface Settings { outputRoot: string; concurrency: number; defaultDuration: number }
export interface LineError { line: number; text: string; err: string }
export type JobStatus = "queued" | "processing" | "done" | "failed";
export interface ClipState { clipId: string; file: string; status: JobStatus; progress: number; attempt: number; error: string; duration: number; size: number }
/** Which files a run leaves in the folder: each clip, only the combined reel, or both. */
export type OutputMode = "clips" | "reel" | "both";
export interface JobView {
  sessionId: string; outputDir: string; mode: OutputMode; clips: ClipState[]; running: boolean;
  concat: { requested: boolean; status: JobStatus | ""; file: string; error: string; duration: number; size: number };
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`/api${path}`, {
    method,
    headers: method === "GET" ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : undefined;
  if (!res.ok) throw new ApiError(res.status, data?.error ?? res.statusText);
  return data as T;
}

export const toInputs = (clips: ClipView[]): ClipInput[] =>
  clips.map(({ id, start, duration, label, approved }) => ({ id, start, duration, label, approved }));

export const api = {
  health: () => request<Health>("GET", "/health"),
  updateYtDlp: () => request<{ output: string }>("POST", "/tools/update-ytdlp"),
  quit: () => request<void>("POST", "/quit"),
  settings: () => request<Settings>("GET", "/settings"),
  saveSettings: (s: Settings) => request<Settings>("PUT", "/settings", s),
  recent: () => request<RecentSession[]>("GET", "/sessions"),
  openVideo: (url: string) => request<SessionView>("POST", "/sessions", { url }),
  session: (id: string) => request<SessionView>("GET", `/sessions/${id}`),
  saveSession: (id: string, body: { leadIn: number; clips: ClipInput[] }) =>
    request<SessionView>("PUT", `/sessions/${id}`, body),
  parseClips: (id: string, text: string) =>
    request<{ session: SessionView; errors: LineError[] }>("POST", `/sessions/${id}/clips/parse`, { text }),
  process: (id: string, mode: OutputMode, reelName: string) =>
    request<JobView>("POST", `/sessions/${id}/process`, { mode, reelName }),
  retry: (id: string, clipId: string) => request<JobView>("POST", `/sessions/${id}/retry/${clipId}`),
  openFolder: (id: string) => request<void>("POST", `/sessions/${id}/open-folder`),
  reveal: (id: string, file: string) => request<void>("POST", `/sessions/${id}/files/${encodeURIComponent(file)}/reveal`),
  fileUrl: (id: string, file: string) => `/api/sessions/${id}/files/${encodeURIComponent(file)}`,
  eventsUrl: (id: string) => `/api/sessions/${id}/events`,
};
