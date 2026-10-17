import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Download, FolderOpen, Pencil, RotateCw } from "lucide-react";
import { useState } from "react";
import { OpenVideoForm } from "@/components/OpenVideoForm";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api, type JobStatus, type JobView, type OutputMode, type SessionView } from "@/lib/api";
import { bytes, length } from "@/lib/format";
import { cn } from "@/lib/utils";

const modes: { value: OutputMode; title: string; detail: (n: number) => string }[] = [
  { value: "clips", title: "Every clip", detail: (n) => `${n} separate file${n === 1 ? "" : "s"}` },
  { value: "reel", title: "Full reel", detail: () => "one file, clips in order" },
  { value: "both", title: "Both", detail: (n) => `${n} clip${n === 1 ? "" : "s"} plus the reel` },
];

/** Choose what to save and start the run. Shown under the clip list while editing. */
export function ProcessPanel({ session, busy, onStarted }: { session: SessionView; busy: boolean; onStarted: () => void }) {
  const id = session.videoId;
  const qc = useQueryClient();
  const [mode, setMode] = useState<OutputMode>("clips");
  const [reelName, setReelName] = useState(session.reelName);
  const process = useMutation({
    mutationFn: () => api.process(id, mode, reelName),
    onSuccess: (v: JobView) => {
      qc.setQueryData(["job", id], v);
      onStarted();
    },
  });

  const n = session.clips.length;
  const unapproved = session.clips.filter((c) => !c.approved).length;
  const errors = session.issues.filter((i) => i.severity === "error").length;
  const reason =
    n === 0 ? "Add at least one clip."
    : errors > 0 ? `Fix ${errors} error${errors > 1 ? "s" : ""} first.`
    : unapproved > 0 ? `Approve ${unapproved} more clip${unapproved > 1 ? "s" : ""} to continue.`
    : null;
  const action = mode === "clips" ? `Process ${n} clip${n === 1 ? "" : "s"}` : mode === "reel" ? "Build the full reel" : `Process ${n} clips and the reel`;

  return (
    <section className="mt-3 rounded-lg border bg-card p-4" aria-labelledby="process-heading">
      <h2 id="process-heading" className="font-heading text-2xl font-bold">Process</h2>
      <p className="mb-3 text-sm text-muted-foreground">Choose what to save, then cut the approved clips.</p>

      <fieldset className="grid gap-2 sm:grid-cols-3">
        <legend className="sr-only">What to save</legend>
        {modes.map((m) => (
          <label
            key={m.value}
            className={cn(
              "cursor-pointer rounded-md border p-3 transition-colors has-focus-visible:ring-2 has-focus-visible:ring-ring",
              mode === m.value ? "border-court bg-accent" : "hover:bg-muted",
            )}
          >
            <input type="radio" name="mode" value={m.value} checked={mode === m.value} onChange={() => setMode(m.value)} className="sr-only" />
            <span className={cn("block font-semibold", mode === m.value && "text-accent-foreground")}>{m.title}</span>
            <span className="block text-xs text-muted-foreground">{m.detail(n)}</span>
          </label>
        ))}
      </fieldset>

      {mode !== "clips" && (
        <div className="mt-3 grid gap-1.5">
          <Label htmlFor="reelName">Reel file name</Label>
          <div className="flex items-center rounded-md border bg-background focus-within:ring-2 focus-within:ring-ring">
            <Input id="reelName" className="border-0 shadow-none focus-visible:ring-0" value={reelName}
              onChange={(e) => setReelName(e.target.value)} placeholder={session.reelName} />
            <span className="pr-3 text-sm text-muted-foreground">.mp4</span>
          </div>
          <p className="text-xs text-muted-foreground">Spaces become underscores and letters are lowercased.</p>
        </div>
      )}

      <div className="mt-4 flex flex-wrap items-center gap-2">
        <Button size="lg" disabled={!session.canProcess || busy || process.isPending} onClick={() => process.mutate()}>
          {busy ? "Processing…" : action}
        </Button>
        {reason && <p className="text-sm text-muted-foreground">{reason}</p>}
      </div>
      {process.error && <p className="mt-2 text-sm text-destructive">{process.error.message}</p>}
    </section>
  );
}

const statusText: Record<JobStatus, string> = { queued: "Waiting", processing: "Cutting", done: "Done", failed: "Failed" };

function StatusChip({ status, progress }: { status: JobStatus; progress?: number }) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium",
        status === "queued" && "bg-muted text-muted-foreground",
        status === "processing" && "bg-accent text-accent-foreground",
        status === "done" && "bg-net/10 text-net",
        status === "failed" && "bg-destructive/10 text-destructive",
      )}
    >
      {status === "processing" && <span className="size-1.5 animate-pulse rounded-full bg-court" aria-hidden />}
      {statusText[status]}
      {status === "processing" && progress !== undefined && <span className="score tabular-nums">{Math.round(progress * 100)}%</span>}
    </span>
  );
}

/** Replaces the clip list while a run is going and after it ends. */
export function JobResults({ session, job, onEdit }: { session: SessionView; job: JobView; onEdit: () => void }) {
  const id = session.videoId;
  const qc = useQueryClient();
  const setJob = (v: JobView) => qc.setQueryData(["job", id], v);
  const retry = useMutation({ mutationFn: (clipId: string) => api.retry(id, clipId), onSuccess: setJob });
  const openFolder = useMutation({ mutationFn: () => api.openFolder(id) });

  const partsOnly = job.mode === "reel";
  const steps = job.clips.length + (job.concat.requested ? 1 : 0);
  const doneSteps = job.clips.filter((c) => c.status === "done").length + (job.concat.status === "done" ? 1 : 0);
  const progress = (job.clips.reduce((sum, c) => sum + (c.status === "done" ? 1 : c.progress), 0) + (job.concat.status === "done" ? 1 : 0)) / Math.max(1, steps);
  const failed = job.clips.filter((c) => c.status === "failed").length + (job.concat.status === "failed" ? 1 : 0);
  const finished = !job.running;
  const files = (partsOnly ? 0 : job.clips.filter((c) => c.status === "done").length) + (job.concat.status === "done" ? 1 : 0);
  const totalLength = job.concat.status === "done" ? job.concat.duration : job.clips.filter((c) => c.status === "done").reduce((s, c) => s + c.duration, 0);

  return (
    <section className="grid content-start gap-3" aria-live="polite">
      <div className={cn("rounded-lg p-4 transition-colors duration-500", finished && failed === 0 ? "bg-board text-white" : "border bg-card")}>
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h2 className="font-heading text-2xl font-bold">
            {!finished ? "Processing" : failed === 0 ? "All done" : `Finished with ${failed} failed`}
          </h2>
          <span className={cn("score text-2xl font-bold", finished && failed === 0 ? "led" : "text-muted-foreground")}>
            {doneSteps}/{steps}
          </span>
        </div>
        <p className={cn("mt-1 text-sm", finished && failed === 0 ? "text-white/70" : "text-muted-foreground")}>
          {!finished
            ? `Cutting ${partsOnly ? "parts for the full reel" : "your clips"}. You can keep this page open or watch the video meanwhile.`
            : failed === 0
              ? `Saved ${files} file${files === 1 ? "" : "s"}, ${length(totalLength)} of video, to ${job.outputDir}.`
              : "Retry the failed items below. Everything else is saved."}
        </p>
        {!finished && (
          <div className="mt-3 h-2 overflow-hidden rounded-full bg-muted" role="progressbar" aria-label="Overall progress"
            aria-valuenow={Math.round(progress * 100)} aria-valuemin={0} aria-valuemax={100}>
            <div className="progress-stripes h-full rounded-full bg-court transition-[width] duration-300" style={{ width: `${Math.max(3, progress * 100)}%` }} />
          </div>
        )}
        <div className="mt-3 flex flex-wrap gap-2">
          <Button variant={finished && failed === 0 ? "secondary" : "outline"} onClick={() => openFolder.mutate()}>
            <FolderOpen className="size-4" /> Open folder
          </Button>
          {finished && (
            <Button variant="ghost" className={cn(failed === 0 && "text-white hover:bg-white/10 hover:text-white")} onClick={onEdit}>
              <Pencil className="size-4" /> Edit these clips
            </Button>
          )}
        </div>
        {[retry.error, openFolder.error].map((e, i) => e && <p key={i} className="mt-2 text-sm text-destructive">{e.message}</p>)}
      </div>

      {finished && (
        <div className="rounded-lg border-2 border-court bg-card p-4">
          <h2 className="font-heading text-xl font-bold">Clip the next match</h2>
          <p className="mb-3 text-sm text-muted-foreground">Paste another video URL to start on it right away.</p>
          <OpenVideoForm autoFocus />
        </div>
      )}

      <ul className="grid gap-2">
        {job.concat.requested && (
          <li className="grid gap-2 rounded-lg border bg-card p-3">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span className="font-heading text-lg font-semibold">Full reel</span>
              <StatusChip status={job.concat.status || "queued"} />
              <span className="min-w-0 flex-1 truncate">{job.concat.file}</span>
              {job.concat.status === "done" && (
                <>
                  <span className="score text-sm text-muted-foreground">{length(job.concat.duration)}<span className="ml-2">{bytes(job.concat.size)}</span></span>
                  <a className="inline-flex items-center gap-1 font-medium text-primary underline-offset-4 hover:underline" href={api.fileUrl(id, job.concat.file)} download>
                    <Download className="size-4" /> Download
                  </a>
                </>
              )}
              {job.concat.status === "failed" && (
                <Button size="sm" variant="outline" disabled={retry.isPending} onClick={() => retry.mutate("concat")}><RotateCw className="size-3.5" /> Retry</Button>
              )}
            </div>
            {job.concat.status === "queued" && finished && <p className="text-xs text-muted-foreground">Waiting for failed clips to be retried.</p>}
            {job.concat.status === "failed" && <p className="text-xs text-destructive">{job.concat.error}</p>}
            {job.concat.status === "done" && <video className="w-full rounded-md" src={api.fileUrl(id, job.concat.file)} controls preload="metadata" />}
          </li>
        )}

        {job.clips.map((c, i) => (
          <li key={c.clipId} className={cn("grid gap-2 rounded-lg border bg-card p-3", c.status === "done" && !partsOnly && "border-l-4 border-l-net")}>
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span className="score w-6 text-lg font-bold text-muted-foreground">{i + 1}</span>
              <StatusChip status={c.status} progress={c.progress} />
              <span className="min-w-0 flex-1 truncate">{partsOnly ? `Part ${i + 1} of ${job.clips.length}` : c.file}</span>
              {c.attempt > 1 && c.status !== "done" && <span className="text-xs text-muted-foreground">attempt {c.attempt} of 3</span>}
              {c.status === "done" && (
                <span className="score text-sm text-muted-foreground">
                  {length(c.duration)}
                  {!partsOnly && c.size > 0 && <span className="ml-2">{bytes(c.size)}</span>}
                </span>
              )}
              {c.status === "failed" && (
                <Button size="sm" variant="outline" disabled={retry.isPending} onClick={() => retry.mutate(c.clipId)}><RotateCw className="size-3.5" /> Retry</Button>
              )}
              {c.status === "done" && !partsOnly && (
                <a className="inline-flex items-center gap-1 font-medium text-primary underline-offset-4 hover:underline" href={api.fileUrl(id, c.file)} download>
                  <Download className="size-4" /> Download
                </a>
              )}
            </div>
            {c.status === "processing" && (
              <div className="h-1.5 overflow-hidden rounded-full bg-muted" aria-hidden>
                <div className="h-full rounded-full bg-court transition-[width] duration-300" style={{ width: `${Math.max(4, c.progress * 100)}%` }} />
              </div>
            )}
            {c.status === "failed" && <p className="text-xs text-destructive">{c.error}</p>}
            {c.status === "done" && !partsOnly && <video className="w-full rounded-md" src={api.fileUrl(id, c.file)} controls preload="metadata" />}
          </li>
        ))}
      </ul>
    </section>
  );
}
