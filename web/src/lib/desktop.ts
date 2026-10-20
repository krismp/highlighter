import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWebview } from "@tauri-apps/api/webview";
import { open } from "@tauri-apps/plugin-dialog";
import { reportError } from "@/lib/errors";

/**
 * The desktop app's native features, for the page it shows. The page is
 * served by the local server, and the app grants it only a file dialog, file
 * drops and its update commands. In a plain browser (development) none exist.
 */
export const inDesktopApp = isTauri();

/** File types the Add videos dialog offers. */
const videoExtensions = ["mp4", "mov", "m4v", "mkv", "webm", "avi", "mts", "m2ts", "3gp", "wmv", "mpg", "mpeg", "ts"];

export const isVideoPath = (p: string) => videoExtensions.includes(p.split(".").pop()?.toLowerCase() ?? "");

/** Asks for one or more videos with the system's file dialog; [] when cancelled. */
export async function pickVideos(): Promise<string[]> {
  const picked = await open({
    title: "Add videos",
    multiple: true,
    directory: false,
    filters: [{ name: "Videos", extensions: videoExtensions }],
  });
  return picked ?? [];
}

export type FileDrag = { type: "enter"; paths: string[] } | { type: "over" } | { type: "drop"; paths: string[] } | { type: "leave" };

/** Calls fn for files dragged over and dropped on the window; returns the unsubscribe. */
export function onFileDrag(fn: (e: FileDrag) => void): () => void {
  if (!inDesktopApp) return () => {};
  let stop: (() => void) | undefined;
  let cancelled = false;
  getCurrentWebview()
    .onDragDropEvent(({ payload }) => {
      if (payload.type === "enter" || payload.type === "drop") fn({ type: payload.type, paths: payload.paths });
      else fn({ type: payload.type });
    })
    .then((un) => (cancelled ? un() : (stop = un)), (err) => reportError("Dropping files on the window won't work", err));
  return () => {
    cancelled = true;
    stop?.();
  };
}
