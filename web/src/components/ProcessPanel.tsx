import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Progress } from "@/components/ui/progress";
import { useJob } from "@/hooks/useJob";
import { api, type JobStatus, type JobView, type SessionView } from "@/lib/api";

const badge: Record<JobStatus, "secondary" | "default" | "destructive" | "outline"> = {
  queued: "outline",
  processing: "secondary",
  done: "default",
  failed: "destructive",
};

export function ProcessPanel({ session }: { session: SessionView }) {
  const id = session.videoId;
  const qc = useQueryClient();
  const job = useJob(id);
  const [concat, setConcat] = useState(false);
  const setJob = (v: JobView) => qc.setQueryData(["job", id], v);
  const process = useMutation({ mutationFn: () => api.process(id, concat), onSuccess: setJob });
  const retry = useMutation({ mutationFn: (clipId: string) => api.retry(id, clipId), onSuccess: setJob });
  const openFolder = useMutation({ mutationFn: () => api.openFolder(id) });

  const unapproved = session.clips.filter((c) => !c.approved).length;
  const errors = session.issues.filter((i) => i.severity === "error").length;
  const reason =
    session.clips.length === 0 ? "Add at least one clip."
    : errors > 0 ? `Fix ${errors} error${errors > 1 ? "s" : ""} first.`
    : unapproved > 0 ? `Review and approve ${unapproved} more clip${unapproved > 1 ? "s" : ""}.`
    : null;
  const running = job?.running ?? false;

  return (
    <Card>
      <CardHeader><CardTitle>Process</CardTitle></CardHeader>
      <CardContent className="grid gap-3">
        <label className="flex items-center gap-2 text-sm">
          <Checkbox checked={concat} onCheckedChange={(v) => setConcat(v === true)} />
          Also combine into highlights_full.mp4
        </label>
        <div className="flex flex-wrap items-center gap-2">
          <Button disabled={!session.canProcess || running || process.isPending} onClick={() => process.mutate()}>
            {running ? "Processing…" : `Process ${session.clips.length} clips`}
          </Button>
          <Button variant="outline" onClick={() => openFolder.mutate()}>Open folder</Button>
        </div>
        {reason && <p className="text-sm text-muted-foreground">{reason}</p>}
        {process.error && <p className="text-sm text-destructive">{process.error.message}</p>}

        {job && (
          <ul className="grid gap-3">
            {job.clips.map((c) => (
              <li key={c.clipId} className="grid gap-1.5 rounded-md border p-2">
                <div className="flex items-center gap-2 text-sm">
                  <Badge variant={badge[c.status]}>{c.status}</Badge>
                  <span className="truncate font-mono">{c.file}</span>
                  {c.attempt > 1 && c.status !== "done" && <span className="text-xs text-muted-foreground">attempt {c.attempt}</span>}
                  {c.status === "failed" && (
                    <Button size="sm" variant="outline" className="ml-auto" disabled={retry.isPending} onClick={() => retry.mutate(c.clipId)}>Retry</Button>
                  )}
                  {c.status === "done" && (
                    <a className="ml-auto text-sm underline" href={api.fileUrl(id, c.file)} download>Download</a>
                  )}
                </div>
                {c.status === "processing" && <Progress value={Math.round(c.progress * 100)} />}
                {c.status === "failed" && <p className="text-xs text-destructive">{c.error}</p>}
                {c.status === "done" && (
                  <video className="w-full rounded" src={api.fileUrl(id, c.file)} controls preload="metadata" />
                )}
              </li>
            ))}
            {job.concat.requested && (
              <li className="flex items-center gap-2 rounded-md border p-2 text-sm">
                <Badge variant={job.concat.status ? badge[job.concat.status] : "outline"}>{job.concat.status || "queued"}</Badge>
                <span className="font-mono">{job.concat.file}</span>
                {job.concat.status === "queued" && !running && <span className="text-xs text-muted-foreground">waiting for failed clips</span>}
                {job.concat.status === "failed" && (
                  <Button size="sm" variant="outline" className="ml-auto" onClick={() => retry.mutate("concat")}>Retry</Button>
                )}
                {job.concat.status === "done" && (
                  <a className="ml-auto underline" href={api.fileUrl(id, job.concat.file)} download>Download</a>
                )}
              </li>
            )}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
