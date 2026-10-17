import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import type { ClipInput, ClipView, Issue } from "@/lib/api";
import { cn } from "@/lib/utils";

const MIN = 1;
const MAX = 120;
const clampDuration = (d: number) => Math.min(MAX, Math.max(MIN, Math.round(d * 10) / 10));

interface Props {
  clip: ClipView;
  issues: Issue[];
  busy: boolean;
  playerReady: boolean;
  previewing: boolean;
  onPreview: () => void;
  onStop: () => void;
  onPatch: (patch: Partial<ClipInput>) => void;
  onRemove: () => void;
}

export function ClipRow({ clip, issues, busy, playerReady, previewing, onPreview, onStop, onPatch, onRemove }: Props) {
  const [label, setLabel] = useState(clip.label);
  const [duration, setDuration] = useState(String(clip.duration));
  useEffect(() => setLabel(clip.label), [clip.label]);
  useEffect(() => setDuration(String(clip.duration)), [clip.duration]);

  const commitDuration = () => {
    const d = Number(duration);
    if (Number.isFinite(d) && d > 0 && clampDuration(d) !== clip.duration) onPatch({ duration: clampDuration(d) });
    else setDuration(String(clip.duration));
  };

  return (
    <Card className={cn("gap-2 p-3", clip.approved && "border-green-600/60")}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="w-8 font-mono text-sm text-muted-foreground">#{clip.index}</span>
        <Button size="sm" variant="ghost" disabled={busy || clip.start < 1} onClick={() => onPatch({ start: Math.max(0, clip.start - 1) })} aria-label="Start one second earlier">−1s</Button>
        <span className="w-16 text-center font-mono">{clip.startText}</span>
        <Button size="sm" variant="ghost" disabled={busy} onClick={() => onPatch({ start: clip.start + 1 })} aria-label="Start one second later">+1s</Button>
        <Input
          className="min-w-32 flex-1"
          placeholder="Label (optional)"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          onBlur={() => label.trim() !== clip.label && onPatch({ label })}
          onKeyDown={(e) => e.key === "Enter" && e.currentTarget.blur()}
        />
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {[-2, 2, 5].map((d) => (
          <Button key={d} size="sm" variant="outline" disabled={busy} onClick={() => onPatch({ duration: clampDuration(clip.duration + d) })}>
            {d > 0 ? `+${d}s` : `−${-d}s`}
          </Button>
        ))}
        <Input
          className="w-20"
          type="number"
          step="0.5"
          min={MIN}
          max={MAX}
          aria-label="Duration in seconds"
          value={duration}
          onChange={(e) => setDuration(e.target.value)}
          onBlur={commitDuration}
          onKeyDown={(e) => e.key === "Enter" && commitDuration()}
        />
        <span className="text-sm text-muted-foreground">s</span>
        {previewing ? (
          <Button size="sm" variant="secondary" onClick={onStop}>■ Stop</Button>
        ) : (
          <Button size="sm" variant="secondary" disabled={!playerReady} onClick={onPreview}>▶ Preview</Button>
        )}
        <label className="ml-auto flex items-center gap-2 text-sm">
          <Checkbox checked={clip.approved} disabled={busy} onCheckedChange={(v) => onPatch({ approved: v === true })} />
          Looks good
        </label>
        <Button size="sm" variant="ghost" disabled={busy} onClick={onRemove}>Remove</Button>
      </div>
      {issues.map((issue, i) => (
        <p key={i} className={cn("text-xs", issue.severity === "error" ? "text-destructive" : "text-amber-600")}>
          {issue.severity === "error" ? "✕ " : "⚠ "}
          {issue.message}
        </p>
      ))}
    </Card>
  );
}
