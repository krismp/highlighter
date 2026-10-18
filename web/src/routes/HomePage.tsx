import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { X } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { ConfirmButton } from "@/components/ConfirmButton";
import { OpenVideoForm } from "@/components/OpenVideoForm";
import { ToolsBanner } from "@/components/ToolsBanner";
import { api } from "@/lib/api";

/** Search-engine simple: the logo, one box for a YouTube link, and recent videos underneath. */
export function HomePage() {
  const qc = useQueryClient();
  const recent = useQuery({ queryKey: ["recent"], queryFn: api.recent, meta: { errorTitle: "Couldn't load recent videos" } });
  // Forgets the video's marked clips; the clip files on disk stay.
  const remove = useMutation({
    meta: { errorTitle: "Couldn't remove the video" },
    mutationFn: api.deleteSession,
    onSuccess: (_, id) => {
      qc.removeQueries({ queryKey: ["session", id] });
      qc.removeQueries({ queryKey: ["job", id] });
      return qc.invalidateQueries({ queryKey: ["recent"] });
    },
  });
  return (
    <div className="mx-auto grid max-w-2xl gap-10 pt-[8vh]">
      <div className="grid justify-items-center gap-6 text-center">
        <div className="flex items-center gap-3">
          <span className="grid size-14 place-items-center rounded-full bg-court text-3xl shadow-md" aria-hidden>🏀</span>
          <h1 className="font-heading text-5xl font-bold tracking-tight">highlighter</h1>
        </div>
        <div className="w-full">
          <OpenVideoForm autoFocus size="lg" />
          <p className="mt-3 text-sm text-muted-foreground">Paste a YouTube link to a match, mark each play, get one clip per play.</p>
        </div>
        <ToolsBanner />
      </div>

      {recent.data && recent.data.length > 0 && (
        <section aria-labelledby="recent-heading">
          <h2 id="recent-heading" className="mb-2 px-3 text-sm font-semibold text-muted-foreground">Recent videos</h2>
          <ul className="divide-y rounded-lg border bg-card">
            {recent.data.map((r) => (
              <li key={r.videoId} className="group flex items-center gap-1 pr-1 hover:bg-muted">
                <Link to="/s/$videoId" params={{ videoId: r.videoId }} className="flex min-w-0 flex-1 justify-between gap-4 px-3 py-2.5">
                  <span className="truncate">{r.title}</span>
                  <span className="shrink-0 text-sm text-muted-foreground">
                    {r.clipCount} clips · {new Date(r.updatedAt).toLocaleDateString()}
                  </span>
                </Link>
                <ConfirmButton question="Remove from recent? Clip files stay saved." confirmLabel="Remove"
                  disabled={remove.isPending} onConfirm={() => remove.mutate(r.videoId)} title="Remove from recent videos"
                  className="shrink-0 text-muted-foreground opacity-60 group-hover:opacity-100 focus-visible:opacity-100">
                  <X className="size-4" /><span className="sr-only">Remove {r.title} from recent videos</span>
                </ConfirmButton>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
