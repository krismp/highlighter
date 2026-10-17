/** 227.4 → "3:47"; 3723 → "1:02:03" (whole seconds, for live player time). */
export function clock(sec: number): string {
  const t = Math.max(0, Math.floor(sec));
  const h = Math.floor(t / 3600);
  const m = Math.floor((t % 3600) / 60);
  const s = String(t % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${s}` : `${m}:${s}`;
}

/** Length of a finished file: "10.0s" under a minute, otherwise "1:23". */
export function length(sec: number): string {
  return sec < 60 ? `${sec.toFixed(1)}s` : clock(Math.round(sec));
}

export function bytes(n: number): string {
  if (n <= 0) return "";
  if (n < 1024 * 1024) return `${Math.max(1, Math.round(n / 1024))} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
