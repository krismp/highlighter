import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { AddClips } from "@/components/AddClips";
import { ClipRow } from "@/components/ClipRow";
import { ProcessPanel } from "@/components/ProcessPanel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useYouTubePlayer } from "@/hooks/useYouTubePlayer";
import { api, toInputs, type ClipInput, type SessionView } from "@/lib/api";

export function SessionPage() {
  const { videoId } = useParams({ from: "/s/$videoId" });
  const qc = useQueryClient();
  const session = useQuery({ queryKey: ["session", videoId], queryFn: () => api.session(videoId) });
  const player = useYouTubePlayer(videoId, session.data !== undefined);
  const setSession = (v: SessionView) => qc.setQueryData(["session", videoId], v);
  const save = useMutation({
    mutationFn: (body: { leadIn: number; clips: ClipInput[] }) => api.saveSession(videoId, body),
    onSuccess: setSession,
  });
  const [leadIn, setLeadIn] = useState("0");
  useEffect(() => {
    if (session.data) setLeadIn(String(session.data.leadIn));
  }, [session.data?.leadIn]);

  if (session.error) return <p className="text-destructive">{session.error.message}</p>;
  const s = session.data;
  if (!s) return <p className="text-muted-foreground">Loading…</p>;

  const busy = save.isPending; // one save at a time so rapid clicks can't lose edits
  const clips = toInputs(s.clips);
  const update = (next: ClipInput[], nextLeadIn = s.leadIn) => save.mutate({ leadIn: nextLeadIn, clips: next });
  const patch = (id: string, p: Partial<ClipInput>) => update(clips.map((c) => (c.id === id ? { ...c, ...p } : c)));
  const unapproved = s.clips.filter((c) => !c.approved).length;
  const commitLeadIn = () => {
    const v = Number(leadIn);
    if (Number.isFinite(v) && v >= 0 && v <= 30 && v !== s.leadIn) update(clips, v);
    else setLeadIn(String(s.leadIn));
  };

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
      <section className="grid content-start gap-4 lg:sticky lg:top-4 lg:self-start">
        <div>
          <h1 className="truncate text-lg font-semibold" title={s.title}>{s.title}</h1>
          <p className="text-sm text-muted-foreground">{s.durationText} · saving to {s.outputDir}</p>
        </div>
        <div ref={player.containerRef} className="aspect-video w-full overflow-hidden rounded-md bg-black [&_iframe]:h-full [&_iframe]:w-full" />
        <Button
          disabled={!player.ready || busy}
          onClick={() => update([...clips, { start: Math.floor(player.currentTime()), duration: 10, label: "", approved: false }])}
        >
          ⚑ Mark play at current time
        </Button>
        <AddClips videoId={videoId} onAdded={setSession} />
        <div className="flex items-center gap-2">
          <Label htmlFor="leadIn">Lead-in (s)</Label>
          <Input id="leadIn" className="w-20" type="number" min={0} max={30} step="0.5" value={leadIn}
            onChange={(e) => setLeadIn(e.target.value)} onBlur={commitLeadIn} onKeyDown={(e) => e.key === "Enter" && commitLeadIn()} />
          <span className="text-xs text-muted-foreground">starts every clip this much earlier; changing it clears approvals</span>
        </div>
        {save.error && <p className="text-sm text-destructive">{save.error.message}</p>}
      </section>

      <section className="grid content-start gap-3">
        <div className="flex items-center justify-between">
          <h2 className="font-semibold">Clips ({s.clips.length})</h2>
          <Button size="sm" variant="outline" disabled={busy || unapproved === 0} onClick={() => update(clips.map((c) => ({ ...c, approved: true })))}>
            Approve all remaining ({unapproved})
          </Button>
        </div>
        {s.clips.length === 0 && <p className="text-sm text-muted-foreground">No clips yet. Mark plays while watching, or paste start times.</p>}
        {s.clips.map((c) => (
          <ClipRow
            key={c.id}
            clip={c}
            issues={s.issues.filter((i) => i.clipId === c.id)}
            busy={busy}
            playerReady={player.ready}
            previewing={player.previewing === c.id}
            onPreview={() => player.preview(c.id, c.effStart, c.effDuration)}
            onStop={player.stopPreview}
            onPatch={(p) => patch(c.id, p)}
            onRemove={() => update(clips.filter((x) => x.id !== c.id))}
          />
        ))}
        <ProcessPanel session={s} />
      </section>
    </div>
  );
}
