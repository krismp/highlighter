import { Film, Pencil, Plus } from "lucide-react";
import { useMutation } from "@tanstack/react-query";
import { Library } from "@/components/combine/Library";
import { LineUp } from "@/components/combine/LineUp";
import { OutputPanel } from "@/components/combine/OutputPanel";
import { ReelStrip } from "@/components/combine/ReelStrip";
import { Button } from "@/components/ui/button";
import { useCombine } from "@/hooks/useCombine";
import { inDesktopApp, pickVideos } from "@/lib/desktop";
import { reportError } from "@/lib/errors";

/** Line up videos from anywhere, in any order, and join them into one MP4. */
export function CombinePage() {
  const c = useCombine();
  const pick = useMutation({
    meta: { errorTitle: "Couldn't open the file dialog" },
    mutationFn: pickVideos,
    onSuccess: (paths) => paths.length > 0 && c.add.mutate(paths),
  });
  const v = c.view;
  if (!v) return c.query.isError ? <p className="text-muted-foreground">Reload the page to try again.</p> : null;

  const job = v.job;
  const locked = job.running || job.status === "done";
  const adding = pick.isPending || c.add.isPending;
  const addVideos = () => {
    if (inDesktopApp) pick.mutate();
    else reportError("Adding files needs the desktop app", "In the browser, pick videos from your highlights below.");
  };

  return (
    <div className="grid gap-6">
      <ReelStrip items={v.items} total={v.total} width={v.width} height={v.height} job={job} />

      {v.items.length === 0 ? (
        <section className="grid justify-items-center gap-3 rounded-xl border-2 border-dashed border-input bg-card/60 px-6 py-12 text-center">
          <span className="grid size-14 place-items-center rounded-full bg-accent text-court"><Film className="size-7" /></span>
          <div>
            <h2 className="font-heading text-3xl font-bold">Drop videos here</h2>
            <p className="mt-1 max-w-md text-muted-foreground">
              Clips from different matches, phone videos, whole games: any mix works. They're joined in the order you set.
            </p>
          </div>
          <Button size="lg" className="h-11 px-5 text-base" disabled={adding} onClick={addVideos}>
            <Plus /> {adding ? "Adding…" : "Add videos…"}
          </Button>
          {!inDesktopApp && <p className="text-xs text-muted-foreground">In the browser, pick from your highlights below.</p>}
        </section>
      ) : (
        <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_23rem]">
          <div className="grid gap-2">
            <LineUp items={v.items} disabled={locked} adding={adding} onAdd={addVideos}
              onReorder={(itemIds) => c.update({ itemIds })}
              onRemove={(id) => c.update({ itemIds: v.items.filter((i) => i.id !== id).map((i) => i.id) })} />
            {job.status === "done" && (
              <Button variant="ghost" className="justify-self-start" disabled={c.dismiss.isPending} onClick={() => c.dismiss.mutate()}>
                <Pencil /> Change these and combine again
              </Button>
            )}
          </div>
          <div className="lg:sticky lg:top-4">
            <OutputPanel c={c} />
          </div>
        </div>
      )}

      {!job.running && <Library items={v.items} disabled={locked || adding} onAdd={(p) => c.add.mutate([p])} />}
    </div>
  );
}
