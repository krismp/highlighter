import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "@tanstack/react-router";
import { Plus, RotateCw, WifiOff } from "lucide-react";
import { useEffect, useState } from "react";
import { AddClips } from "@/components/AddClips";
import { ClipRow } from "@/components/ClipRow";
import { ErrorPanel } from "@/components/ErrorPanel";
import { JobResults, ProcessPanel } from "@/components/ProcessPanel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useConnectivity } from "@/hooks/useConnectivity";
import { useJob } from "@/hooks/useJob";
import { useYouTubePlayer } from "@/hooks/useYouTubePlayer";
import { api, toInputs, type ClipInput, type SessionView } from "@/lib/api";
import { clock } from "@/lib/format";

export function SessionPage() {
  const { videoId } = useParams({ from: "/s/$videoId" });
  // Keyed so opening the next video from the results starts with fresh page state.
  return <Session key={videoId} videoId={videoId} />;
}

function Session({ videoId }: { videoId: string }) {
  const qc = useQueryClient();
  const job = useJob(videoId);
  // The results replace the clip list while a run is going, and stay after it
  // ends until "Edit these clips".
  const [resultsOpen, setResultsOpen] = useState(false);
  const showResults = job !== null && (resultsOpen || job.running);
  const session = useQuery({ queryKey: ["session", videoId], queryFn: () => api.session(videoId), meta: { inline: true } });
  const player = useYouTubePlayer(videoId, session.data !== undefined);
  const net = useConnectivity();
  const setSession = (v: SessionView) => qc.setQueryData(["session", videoId], v);
  // Writes run one at a time (scope) and each body is built from the latest cached
  // session when it runs, so queued edits never overwrite one another.
  const save = useMutation({
    meta: { errorTitle: "Couldn't save your clips" },
    scope: { id: `session-${videoId}` },
    mutationFn: (build: (s: SessionView) => { leadIn: number; clips: ClipInput[] }) => {
      const latest = qc.getQueryData<SessionView>(["session", videoId]);
      if (!latest) throw new Error("session not loaded");
      return api.saveSession(videoId, build(latest));
    },
    onSuccess: setSession,
  });
  const settings = useQuery({ queryKey: ["settings"], queryFn: api.settings, meta: { errorTitle: "Couldn't load settings" } });
  const defaultLength = settings.data?.defaultDuration ?? 10;
  const [lengthInput, setLengthInput] = useState("");
  useEffect(() => {
    if (settings.data) setLengthInput(String(settings.data.defaultDuration));
  }, [settings.data?.defaultDuration]);
  const saveDefault = useMutation({
    meta: { errorTitle: "Couldn't save the clip length" },
    mutationFn: (defaultDuration: number) => api.saveSettings({ ...settings.data!, defaultDuration }),
    onSuccess: (st) => qc.setQueryData(["settings"], st),
  });
  const [leadIn, setLeadIn] = useState("0");
  useEffect(() => {
    if (session.data) setLeadIn(String(session.data.leadIn));
  }, [session.data?.leadIn]);

  // Adjusting the clip that is playing moves its end point; playback carries on.
  const playing = session.data?.clips.find((c) => c.id === player.previewing);
  const { extend } = player;
  useEffect(() => {
    if (playing) extend(playing.id, playing.effStart, playing.effDuration);
  }, [playing?.id, playing?.effStart, playing?.effDuration, extend]);

  if (session.error) return <ErrorPanel title="Couldn't load this video's clips" error={session.error} onRetry={() => session.refetch()} />;
  const s = session.data;
  if (!s) return <p className="text-muted-foreground">Loading…</p>;

  const update = (build: (cur: SessionView) => { leadIn?: number; clips: ClipInput[] }) =>
    save.mutate((cur) => ({ leadIn: cur.leadIn, ...build(cur) }));
  const patch = (id: string, p: Partial<ClipInput>) =>
    update((cur) => ({ clips: toInputs(cur.clips).map((c) => (c.id === id ? { ...c, ...p } : c)) }));
  const approved = s.clips.filter((c) => c.approved).length;
  const unapproved = s.clips.length - approved;
  const commitLeadIn = () => {
    const v = Number(leadIn);
    if (leadIn.trim() !== "" && Number.isFinite(v) && v >= 0 && v <= 30 && v !== s.leadIn) update((cur) => ({ leadIn: v, clips: toInputs(cur.clips) }));
    else setLeadIn(String(s.leadIn));
  };
  const addAtPlayhead = () => {
    const start = Math.floor(player.currentTime());
    update((cur) => ({ clips: [...toInputs(cur.clips), { start, duration: defaultLength, label: "", approved: false }] }));
  };
  const commitDefaultLength = () => {
    const v = Math.round(Number(lengthInput) * 10) / 10;
    if (settings.data && lengthInput.trim() !== "" && v >= 1 && v <= 120 && v !== defaultLength) saveDefault.mutate(v);
    else setLengthInput(String(defaultLength));
  };

  return (
    <div className="grid gap-8 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)]">
      <section className="grid content-start gap-4 lg:sticky lg:top-4 lg:self-start">
        <div>
          <h1 className="font-heading text-3xl leading-tight font-bold text-balance" title={s.title}>{s.title}</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {s.durationText} long. Clips are saved to <span className="break-all text-foreground">{s.outputDir}</span>
          </p>
        </div>

        <div className="overflow-hidden rounded-lg bg-board shadow-lg">
          <div className="relative">
            <div ref={player.containerRef} className="aspect-video w-full bg-black [&_iframe]:h-full [&_iframe]:w-full" />
            {(player.error || net.online === false) && !player.ready && (
              <div className="absolute inset-0 grid place-items-center bg-board p-6 text-center text-white">
                <div className="grid max-w-sm justify-items-center gap-3">
                  <WifiOff className="size-8 text-white/60" aria-hidden />
                  <p className="text-sm">{net.online === false ? "The video preview needs an internet connection." : player.error}</p>
                  <Button size="sm" variant="secondary" onClick={player.retry}><RotateCw className="size-3.5" /> Try again</Button>
                </div>
              </div>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-3 px-3 py-2.5 text-white">
            <span className="score led w-20 text-3xl font-bold" aria-label="Current video time">{clock(player.playhead)}</span>
            <p className="min-w-40 flex-1 text-sm text-white/70">Pause where a play starts, then add it as a clip.</p>
            <Button disabled={!player.ready} onClick={addAtPlayhead}>
              <Plus className="size-4" /> Add clip at {clock(player.playhead)}
            </Button>
          </div>
        </div>

        {!showResults && <div className="rounded-lg border bg-card p-4">
          <h2 className="font-heading text-lg font-semibold">Paste start times</h2>
          <p className="mb-3 text-sm text-muted-foreground">One per line, with an optional label after the time.</p>
          <AddClips videoId={videoId} onAdded={setSession} />
          <div className="mt-4 flex flex-wrap items-center gap-2 border-t pt-4">
            <Label htmlFor="defaultLength">New clips last</Label>
            <Input id="defaultLength" className="w-20" type="number" min={1} max={120} step="0.5" value={lengthInput}
              onChange={(e) => setLengthInput(e.target.value)} onBlur={commitDefaultLength} onKeyDown={(e) => e.key === "Enter" && commitDefaultLength()} />
            <span className="text-sm text-muted-foreground">seconds. Used for pasted clips and "Add clip at". Existing clips keep their length.</span>
          </div>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <Label htmlFor="leadIn">Lead-in</Label>
            <Input id="leadIn" className="w-20" type="number" min={0} max={30} step="0.5" value={leadIn}
              onChange={(e) => setLeadIn(e.target.value)} onBlur={commitLeadIn} onKeyDown={(e) => e.key === "Enter" && commitLeadIn()} />
            <span className="text-sm text-muted-foreground">seconds added before every clip. Changing it clears approvals.</span>
          </div>
        </div>}
      </section>

      {showResults ? (
        <JobResults session={s} job={job} onEdit={() => setResultsOpen(false)} />
      ) : (
        <section className="grid content-start gap-3">
          <div className="flex flex-wrap items-end justify-between gap-2">
            <div>
              <h2 className="font-heading text-2xl font-bold">Clips</h2>
              <p className="text-sm text-muted-foreground">
                {s.clips.length === 0 ? "None yet" : `${approved} of ${s.clips.length} approved`}
              </p>
            </div>
            <Button size="sm" variant="outline" disabled={unapproved === 0}
              onClick={() => update((cur) => ({ clips: toInputs(cur.clips).map((c) => ({ ...c, approved: true })) }))}>
              Approve the other {unapproved}
            </Button>
          </div>
          {s.clips.length > 0 && (
            <div className="h-1.5 overflow-hidden rounded-full bg-muted" aria-hidden>
              <div className="h-full bg-net transition-[width] duration-500" style={{ width: `${(approved / s.clips.length) * 100}%` }} />
            </div>
          )}
          {s.clips.length === 0 && (
            <p className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
              Add a clip from the player, or paste the start times of each play.
            </p>
          )}
          {s.clips.map((c) => (
            <ClipRow
              key={c.id}
              clip={c}
              issues={s.issues.filter((i) => i.clipId === c.id)}
              playerReady={player.ready}
              previewing={player.previewing === c.id}
              playhead={player.playhead}
              range={player.previewing === c.id ? player.range : null}
              onPreview={() => player.preview(c.id, c.effStart, c.effDuration)}
              onStop={player.stopPreview}
              onPatch={(p) => patch(c.id, p)}
              onRemove={() => update((cur) => ({ clips: toInputs(cur.clips).filter((x) => x.id !== c.id) }))}
            />
          ))}
          <ProcessPanel session={s} busy={job?.running ?? false} onStarted={() => setResultsOpen(true)} />
        </section>
      )}
    </div>
  );
}
