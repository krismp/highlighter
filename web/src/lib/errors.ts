import { useSyncExternalStore } from "react";
import { ApiError } from "@/lib/api";

/** One error shown to the user as a closable alert. */
export interface ReportedError {
  id: number;
  title: string;    // what the user was doing: "Couldn't save settings"
  message: string;  // what went wrong, as readable as we can make it
  detail: string;   // technical context for bug reports (request, stack)
  time: Date;
  count: number;    // the same error repeating bumps this instead of stacking alerts
}

let errors: ReportedError[] = [];
let nextId = 1;
const listeners = new Set<() => void>();
const emit = () => listeners.forEach((l) => l());

/** Describes any thrown value: ApiErrors carry the request, Errors their stack. */
function describe(err: unknown): { message: string; detail: string } {
  if (err instanceof ApiError) return { message: err.message, detail: err.request };
  if (err instanceof Error) return { message: err.message || err.name, detail: err.stack ?? "" };
  if (typeof err === "string") return { message: err, detail: "" };
  try {
    return { message: JSON.stringify(err), detail: "" };
  } catch {
    return { message: String(err), detail: "" };
  }
}

/** Shows an error to the user. Every failure in the app should end up here or on screen. */
export function reportError(title: string, err: unknown, extraDetail = "") {
  const { message, detail } = describe(err);
  const full = [extraDetail, detail].filter(Boolean).join("\n");
  const same = errors.find((e) => e.title === title && e.message === message);
  if (same) {
    errors = errors.map((e) => (e === same ? { ...e, count: e.count + 1, time: new Date() } : e));
  } else {
    errors = [...errors, { id: nextId++, title, message, detail: full, time: new Date(), count: 1 }].slice(-5);
  }
  emit();
}

export function dismissError(id: number) {
  errors = errors.filter((e) => e.id !== id);
  emit();
}

export function useReportedErrors(): ReportedError[] {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => errors,
  );
}

/** App facts added to every copied report; filled in from /api/health. */
export const reportContext = { version: "", os: "" };

/** Plain-text report for pasting into a bug report. */
export function formatReport(e: { title: string; message: string; detail: string; time: Date }): string {
  return [
    `${e.title}: ${e.message}`,
    e.detail && `\n${e.detail}`,
    `\nTime: ${e.time.toISOString()}`,
    `App: highlighter ${reportContext.version || "unknown version"} (${reportContext.os || navigator.platform})`,
    `Page: ${location.pathname}`,
    `Browser: ${navigator.userAgent}`,
  ].filter(Boolean).join("\n");
}

/** Copies text, falling back to a hidden textarea where the Clipboard API is unavailable. */
export async function copyText(text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text);
    return;
  } catch {
    const area = document.createElement("textarea");
    area.value = text;
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.select();
    const ok = document.execCommand("copy");
    area.remove();
    if (!ok) throw new Error("Copy is blocked here. Select the text and copy it instead.");
  }
}
