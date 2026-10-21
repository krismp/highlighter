import { Channel, invoke } from "@tauri-apps/api/core";
import { useSyncExternalStore } from "react";
import { inDesktopApp } from "@/lib/desktop";

/** A newer version on the release feed. */
export interface UpdateInfo { version: string; currentVersion: string; notes: string | null; date: number | null }

/** Asks the desktop app whether a newer version is out; null when this one is the latest. */
export const checkForUpdate = () => invoke<UpdateInfo | null>("check_update");

type DownloadEvent = { event: "progress"; downloaded: number; total: number | null } | { event: "installing" };

/** Where an install is: shared by the banner and the Settings card. */
export type InstallState =
  | { phase: "idle" }
  | { phase: "downloading"; downloaded: number; total: number | null }
  | { phase: "installing" }
  | { phase: "failed"; error: string };

let state: InstallState = { phase: "idle" };
const listeners = new Set<() => void>();
const set = (s: InstallState) => {
  state = s;
  listeners.forEach((l) => l());
};

export function useInstallState(): InstallState {
  return useSyncExternalStore((l) => {
    listeners.add(l);
    return () => listeners.delete(l);
  }, () => state);
}

/**
 * Downloads and installs the update found by the last check, then the app
 * restarts into the new version (the page goes away with it). Rejections end
 * in the "failed" state.
 */
export async function installUpdate(): Promise<void> {
  if (!inDesktopApp || state.phase === "downloading" || state.phase === "installing") return;
  set({ phase: "downloading", downloaded: 0, total: null });
  const onEvent = new Channel<DownloadEvent>();
  onEvent.onmessage = (e) => set(e.event === "installing" ? { phase: "installing" } : { phase: "downloading", downloaded: e.downloaded, total: e.total });
  try {
    await invoke("install_update", { onEvent });
  } catch (err) {
    set({ phase: "failed", error: err instanceof Error ? err.message : String(err) });
  }
}

/** "Later" hides the banner for that version only. */
const dismissedKey = "highlighter.update.dismissed";
export function dismissedVersion(): string {
  try {
    return localStorage.getItem(dismissedKey) ?? "";
  } catch {
    return "";
  }
}
export function dismissVersion(version: string) {
  try {
    localStorage.setItem(dismissedKey, version);
  } catch {
    // private mode: the banner just comes back next launch
  }
}
