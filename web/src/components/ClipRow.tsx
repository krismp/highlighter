import { Check, Play, Square, Trash2 } from "lucide-react";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { ClipInput, ClipView, Issue } from "@/lib/api";
import { cn } from "@/lib/utils";

const MIN = 1;
const MAX = 120;
const clampDuration = (d: number) => Math.min(MAX, Math.max(MIN, Math.round(d * 10) / 10));

interface Props {
  clip: ClipView;
  issues: Issue[];
  playerReady: boolean;
  previewing: boolean;
  /** Seconds into the video and the running preview's bounds, while previewing. */
  playhead: number;
  range: { start: number; end: number } | null;
  onPreview: () => void;
  onStop: () => void;
  onPatch: (patch: Partial<ClipInput>) => void;
  onRemove: () => void;
}

export function ClipRow({ clip, issues, playerReady, previewing, playhead, range, onPreview, onStop, onPatch, onRemove }: Props) {
  const [label, setLabel] = useState(clip.label);
  const [duration, setDuration] = useState(String(clip.duration));
  useEffect(() => setLabel(clip.label), [clip.label]);
  useEffect(() => setDuration(String(clip.duration)), [clip.duration]);

  const commitDuration = () => {
    const d = Number(duration);
    if (Number.isFinite(d) && d > 0 && clampDuration(d) !== clip.duration) onPatch({ duration: clampDuration(d) });
    else setDuration(String(clip.duration));
  };

  const live = previewing && range !== null;
  const span = live ? range.end - range.start : 0;
  const elapsed = live ? Math.min(span, Math.max(0, playhead - range.start)) : 0;
  const remaining = Math.max(0, span - elapsed);

  return (
    <article
      aria-label={`Clip ${clip.index}`}
      className={cn(
        "relative overflow-hidden rounded-lg border bg-card p-3 transition-[background-color,box-shadow,color] duration-300",
        clip.approved && !live && "border-l-4 border-l-net",
        live && "border-transparent bg-board text-white shadow-[0_0_0_2px_var(--court),0_12px_32px_-12px_rgb(234_91_12_/_0.6)]",
      )}
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <span className={cn("score w-9 text-2xl font-bold", live ? "text-court" : "text-muted-foreground")}>{clip.index}</span>
        <div className="flex items-center">
          <Button size="sm" variant="ghost" className={cn(live && "text-white hover:bg-white/10 hover:text-white")} disabled={clip.start < 1}
            onClick={() => onPatch({ start: Math.max(0, clip.start - 1) })} aria-label="Start one second earlier">−1s</Button>
          <span className="score w-16 text-center text-2xl font-semibold">{clip.startText}</span>
          <Button size="sm" variant="ghost" className={cn(live && "text-white hover:bg-white/10 hover:text-white")}
            onClick={() => onPatch({ start: clip.start + 1 })} aria-label="Start one second later">+1s</Button>
        </div>
        <Input
          className={cn("min-w-32 flex-1", live && "border-white/20 bg-white/5 text-white placeholder:text-white/40")}
          placeholder="Label, e.g. 3PT"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          onBlur={() => label.trim() !== clip.label && onPatch({ label })}
          onKeyDown={(e) => e.key === "Enter" && e.currentTarget.blur()}
        />
        {live && (
          <div className="flex items-center gap-2 rounded-md bg-black/30 px-2.5 py-1" aria-live="off">
            <span className="flex h-4 items-end gap-0.5" aria-hidden>
              <span className="eq-bar h-full w-1 rounded-sm bg-court" />
              <span className="eq-bar h-full w-1 rounded-sm bg-court" />
              <span className="eq-bar h-full w-1 rounded-sm bg-court" />
            </span>
            <span className="score led text-2xl font-bold" title="Seconds left in this clip">{remaining.toFixed(1).padStart(4, "0")}</span>
          </div>
        )}
      </div>

      <div className="mt-2 flex flex-wrap items-center gap-2">
        <label
          className={cn(
            "flex h-9 items-center gap-1 rounded-md border bg-background px-2 shadow-xs focus-within:ring-2 focus-within:ring-ring",
            live && "border-white/25 bg-white/10",
          )}
        >
          <span className={cn("text-xs", live ? "text-white/60" : "text-muted-foreground")}>Length</span>
          <input
            className={cn("score w-12 bg-transparent text-right text-lg font-semibold outline-none [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none", live && "text-white")}
            type="number" step="0.5" min={MIN} max={MAX} inputMode="decimal"
            aria-label="Clip length in seconds"
            value={duration}
            onChange={(e) => setDuration(e.target.value)}
            onBlur={commitDuration}
            onKeyDown={(e) => e.key === "Enter" && commitDuration()}
          />
          <span className={cn("text-sm", live ? "text-white/60" : "text-muted-foreground")}>s</span>
        </label>
        <div className="flex items-center gap-1" role="group" aria-label="Adjust length">
          {[-2, 2, 5].map((d) => (
            <button
              key={d}
              type="button"
              aria-label={d > 0 ? `Make clip ${d} seconds longer` : `Make clip ${-d} seconds shorter`}
              onClick={() => onPatch({ duration: clampDuration(clip.duration + d) })}
              className={cn(
                "score h-7 rounded-full px-3 text-sm font-semibold transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring active:scale-95",
                live
                  ? "bg-white/10 text-white hover:bg-court"
                  : d < 0
                    ? "bg-secondary text-secondary-foreground hover:bg-muted-foreground/20"
                    : "bg-accent text-accent-foreground hover:bg-court hover:text-white",
              )}
            >
              {d > 0 ? `+${d}s` : `−${-d}s`}
            </button>
          ))}
        </div>

        {live ? (
          <Button size="sm" variant="secondary" onClick={onStop}><Square className="size-3.5 fill-current" /> Stop</Button>
        ) : (
          <Button size="sm" disabled={!playerReady} onClick={onPreview}><Play className="size-3.5 fill-current" /> Preview</Button>
        )}

        <Button
          size="sm"
          variant={clip.approved ? "default" : "outline"}
          aria-pressed={clip.approved}
          className={cn("ml-auto", clip.approved && "bg-net text-white hover:bg-net/90", live && !clip.approved && "border-white/30 bg-transparent text-white hover:bg-white/10 hover:text-white")}
          onClick={() => onPatch({ approved: !clip.approved })}
        >
          <Check className="size-3.5" /> {clip.approved ? "Approved" : "Looks good"}
        </Button>
        <Button size="icon-sm" variant="ghost" aria-label={`Remove clip ${clip.index}`}
          className={cn("text-muted-foreground hover:text-destructive", live && "text-white/60 hover:bg-white/10")}
          onClick={onRemove}>
          <Trash2 className="size-4" />
        </Button>
      </div>

      {issues.map((issue, i) => (
        <p key={i} className={cn("mt-2 text-xs", issue.severity === "error" ? "text-destructive" : live ? "text-led" : "text-amber-700")}>
          {issue.severity === "error" ? "✕ " : "⚠ "}
          {issue.message}
        </p>
      ))}

      {live && (
        <div className="absolute inset-x-0 bottom-0 h-1 bg-white/10" aria-hidden>
          <div className="h-full bg-court transition-[width] duration-100 ease-linear" style={{ width: `${span ? (elapsed / span) * 100 : 0}%` }} />
        </div>
      )}
    </article>
  );
}
