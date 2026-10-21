import { CircleCheck, FolderOpen, Plus, RotateCw, Search, Trash2, Zap } from "lucide-react";
import { useEffect, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/ConfirmButton";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useFileManager, useTrashName } from "@/hooks/useFileManager";
import type { useCombine } from "@/hooks/useCombine";
import { api, type CombineSize, type CombineView, type Transition } from "@/lib/api";
import { reportError } from "@/lib/errors";
import { bytes, clock, length } from "@/lib/format";
import { cn } from "@/lib/utils";

const sizes: { value: CombineSize; title: string; w: number; h: number }[] = [
  { value: "auto", title: "Match first video", w: 16, h: 9 },
  { value: "1080p", title: "1080p", w: 16, h: 9 },
  { value: "720p", title: "720p", w: 16, h: 9 },
  { value: "vertical", title: "Vertical", w: 9, h: 16 },
  { value: "square", title: "Square", w: 1, h: 1 },
];

const transitions: { value: Transition; title: string; detail: string }[] = [
  { value: "cut", title: "Cut", detail: "straight to the next video" },
  { value: "fade", title: "Fade", detail: "through black, half a second" },
];

type Combine = ReturnType<typeof useCombine>;

/** Name, size and transition, then the Combine button; progress and the result take its place. */
export function OutputPanel({ c }: { c: Combine }) {
  const v = c.view!;
  const job = v.job;
  if (job.running) return <Running c={c} />;
  if (job.status === "done") return <Done c={c} />;
  return <Choices c={c} v={v} />;
}

function Choices({ c, v }: { c: Combine; v: CombineView }) {
  const [name, setName] = useState(v.name);
  useEffect(() => setName(v.name), [v.name]);
  const saveName = () => name.trim() !== v.name && c.update({ name: name.trim() });

  const n = v.items.length;
  const missing = v.items.filter((i) => i.missing).length;
  const reason = n < 2 ? (n === 0 ? "Add the videos you want to join." : "Add at least one more video.")
    : missing > 0 ? `Remove or re-add ${missing === 1 ? "the missing video" : `the ${missing} missing videos`} first.` : null;
  const job = v.job;

  return (
    <section aria-labelledby="output-heading" className="grid gap-4 rounded-xl border bg-card p-4">
      <h2 id="output-heading" className="font-heading text-2xl font-bold">Your video</h2>

      {job.status === "failed" && (
        <p role="alert" className="rounded-md bg-destructive/10 p-2 text-sm text-destructive">
          The last try failed: {job.error}
        </p>
      )}
      {job.status === "canceled" && <p className="rounded-md bg-muted p-2 text-sm text-muted-foreground">Cancelled. Nothing was saved.</p>}

      <div className="grid gap-1.5">
        <Label htmlFor="combineName">File name</Label>
        <div className="flex items-center rounded-md border bg-background focus-within:ring-2 focus-within:ring-ring">
          <Input id="combineName" className="border-0 shadow-none focus-visible:ring-0" value={name} placeholder={v.defaultName}
            onChange={(e) => setName(e.target.value)} onBlur={saveName} onKeyDown={(e) => e.key === "Enter" && saveName()} />
          <span className="pr-3 text-sm text-muted-foreground">.mp4</span>
        </div>
      </div>

      <fieldset className="grid gap-1.5">
        <legend className="mb-1.5 text-sm font-medium">Size</legend>
        <div className="grid grid-cols-5 gap-1.5">
          {sizes.map((s) => {
            const on = v.size === s.value;
            return (
              <label key={s.value} title={s.title}
                className={cn("grid cursor-pointer justify-items-center gap-1.5 rounded-md border px-1 py-2 text-center text-xs transition-colors has-focus-visible:ring-2 has-focus-visible:ring-ring",
                  on ? "border-court bg-accent font-semibold text-accent-foreground" : "hover:bg-muted")}>
                <input type="radio" name="size" className="sr-only" checked={on} onChange={() => c.update({ size: s.value })} />
                <span className="grid h-6 place-items-center" aria-hidden>
                  <span className={cn("block rounded-[3px] border-2", on ? "border-court bg-court/20" : "border-current opacity-50")}
                    style={{ width: (s.w / Math.max(s.w, s.h)) * 22, height: (s.h / Math.max(s.w, s.h)) * 22 }} />
                </span>
                <span className="leading-tight">{s.value === "auto" ? "Like #1" : s.title}</span>
              </label>
            );
          })}
        </div>
        <p className="text-xs text-muted-foreground">
          {v.size === "auto" ? "Same size as the first video" : sizes.find((s) => s.value === v.size)?.title}: {v.width}×{v.height}. Videos
          of another shape get black bars, never stretched or cropped.
        </p>
      </fieldset>

      <fieldset className="grid gap-1.5">
        <legend className="mb-1.5 text-sm font-medium">Between videos</legend>
        <div className="grid grid-cols-2 gap-1.5">
          {transitions.map((t) => {
            const on = v.transition === t.value;
            return (
              <label key={t.value}
                className={cn("cursor-pointer rounded-md border p-2.5 transition-colors has-focus-visible:ring-2 has-focus-visible:ring-ring",
                  on ? "border-court bg-accent" : "hover:bg-muted")}>
                <input type="radio" name="transition" className="sr-only" checked={on} onChange={() => c.update({ transition: t.value })} />
                <span className={cn("block font-semibold", on && "text-accent-foreground")}>{t.title}</span>
                <span className="block text-xs text-muted-foreground">{t.detail}</span>
              </label>
            );
          })}
        </div>
      </fieldset>

      {v.copy && n > 1 && (
        <p className="flex gap-2 rounded-md bg-net/10 p-2 text-sm text-net">
          <Zap className="mt-0.5 size-4 shrink-0" />
          These videos match, so they're joined as they are: done in seconds, no quality lost.
        </p>
      )}

      <div className="grid gap-2">
        <Button size="lg" className="h-11 text-base" disabled={!!reason || c.start.isPending} onClick={() => c.start.mutate()}>
          {job.status === "failed" ? <><RotateCw /> Try again</> : `Combine ${n > 1 ? `${n} videos` : "videos"}`}
        </Button>
        <p className="text-xs text-muted-foreground">
          {reason ?? <>Saved to <span className="break-all">{v.outputDir}</span>. Your videos aren't changed.</>}
        </p>
      </div>
    </section>
  );
}

function Running({ c }: { c: Combine }) {
  const job = c.view!.job;
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(t);
  }, []);
  const elapsed = (now - new Date(job.startedAt).getTime()) / 1000;
  const left = job.progress > 0.03 && elapsed > 3 ? (elapsed * (1 - job.progress)) / job.progress : null;
  const pct = Math.round(job.progress * 100);
  return (
    <section aria-live="polite" className="grid gap-3 rounded-xl border bg-card p-4">
      <h2 className="font-heading text-2xl font-bold">Combining</h2>
      <p className="text-sm text-muted-foreground">
        {job.copy ? "Joining your videos as they are." : `Fitting ${job.count} videos to ${job.width}×${job.height} and joining them.`} You can
        keep using highlighter meanwhile.
      </p>
      <div className="flex items-baseline justify-between">
        <span className="score text-4xl font-bold tabular-nums">{job.copy ? "…" : `${pct}%`}</span>
        <span className="text-sm text-muted-foreground tabular-nums">{left !== null ? `about ${clock(left)} left` : job.progress > 0 ? "working out the time left…" : "starting…"}</span>
      </div>
      <div className="h-2.5 overflow-hidden rounded-full bg-muted" role="progressbar" aria-label="Combine progress" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}>
        <div className="progress-stripes h-full rounded-full bg-court transition-[width] duration-500" style={{ width: `${Math.max(3, job.copy ? 100 : pct)}%` }} />
      </div>
      <p className="truncate text-sm">{job.file}</p>
      <Button variant="outline" disabled={c.cancel.isPending} onClick={() => c.cancel.mutate()}>Cancel</Button>
    </section>
  );
}

function Done({ c }: { c: Combine }) {
  const job = c.view!.job;
  const fileManager = useFileManager();
  const trashName = useTrashName();
  const reveal = useMutation({ meta: { errorTitle: "Couldn't show the file" }, mutationFn: api.revealCombined });
  const openFolder = useMutation({ meta: { errorTitle: "Couldn't open the folder" }, mutationFn: api.openCombinedFolder });
  return (
    <section aria-live="polite" className="grid gap-3 rounded-xl bg-board p-4 text-white">
      <div className="flex items-baseline justify-between gap-2">
        <h2 className="font-heading text-2xl font-bold">{job.trashed ? `Moved to ${trashName}` : "Combined"}</h2>
        <span className="score led text-2xl font-bold">{length(job.duration)}</span>
      </div>
      {!job.trashed && (
        <>
          <video key={job.file} src={api.combinedUrl(job.file)} controls preload="metadata" className="w-full rounded-md bg-black"
            onError={() => reportError("Couldn't play the combined video", `It is still saved in ${job.dir}.`)} />
          <p className="flex items-start gap-1.5 text-sm text-white/70">
            <CircleCheck className="mt-0.5 size-4 shrink-0 text-led" />
            <span className="min-w-0 break-all">{job.file} · {job.width}×{job.height} · {bytes(job.size)}</span>
          </p>
          <div className="flex flex-wrap gap-1.5">
            <Button variant="secondary" disabled={reveal.isPending} onClick={() => reveal.mutate()}><Search /> Show in {fileManager}</Button>
            <Button variant="ghost" className="text-white hover:bg-white/10 hover:text-white" onClick={() => openFolder.mutate()}><FolderOpen /> Open folder</Button>
            <ConfirmButton question={`Move it to the ${trashName}?`} confirmLabel={`Move to ${trashName}`} disabled={c.trash.isPending}
              onConfirm={() => c.trash.mutate()} title={`Move the combined video to the ${trashName}`}
              className="text-white/70 hover:bg-white/10 hover:text-white [&_.text-muted-foreground]:text-white/70">
              <Trash2 /> Move to {trashName}
            </ConfirmButton>
          </div>
        </>
      )}
      <Button size="lg" className="mt-1 h-11" disabled={c.clear.isPending} onClick={() => c.clear.mutate()}>
        <Plus /> Start a new combination
      </Button>
      <p className="-mt-1 text-xs text-white/50">Clears the list. Every file stays where it is.</p>
    </section>
  );
}
