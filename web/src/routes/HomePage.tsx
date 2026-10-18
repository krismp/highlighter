import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { OpenVideoForm } from "@/components/OpenVideoForm";
import { ToolsBanner } from "@/components/ToolsBanner";
import { api } from "@/lib/api";

/** Search-engine simple: the logo, one box for a YouTube link, and recent videos underneath. */
export function HomePage() {
  const recent = useQuery({ queryKey: ["recent"], queryFn: api.recent, meta: { errorTitle: "Couldn't load recent videos" } });
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
              <li key={r.videoId}>
                <Link to="/s/$videoId" params={{ videoId: r.videoId }} className="flex justify-between gap-4 px-3 py-2.5 hover:bg-muted">
                  <span className="truncate">{r.title}</span>
                  <span className="shrink-0 text-sm text-muted-foreground">
                    {r.clipCount} clips · {new Date(r.updatedAt).toLocaleDateString()}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
