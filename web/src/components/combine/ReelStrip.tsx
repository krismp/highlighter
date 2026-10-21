import { api, type CombineItem, type CombineJob } from "@/lib/api";
import { clock } from "@/lib/format";
import { cn } from "@/lib/utils";

/**
 * The finished video at a glance, on the scoreboard: one segment per video,
 * as wide as its share of the total length, with the total in LED digits.
 * While combining, the played part lights up and a playhead sweeps across.
 */
export function ReelStrip({ items, total, width, height, job }: {
  items: CombineItem[]; total: number; width: number; height: number; job: CombineJob;
}) {
  const running = job.running;
  const progress = job.status === "done" ? 1 : running ? job.progress : 0;
  return (
    <section aria-label="The combined video" className="rounded-xl bg-board p-3 text-white shadow-lg sm:p-4">
      <div className="mb-3 flex flex-wrap items-end justify-between gap-x-6 gap-y-1">
        <div>
          <h1 className="font-heading text-3xl font-bold leading-none sm:text-4xl">Combine videos</h1>
          <p className="mt-1 text-sm text-white/60">
            {items.length === 0 ? "Join clips from different matches, or any videos, into one."
              : `${items.length} video${items.length === 1 ? "" : "s"} in this order, ${width}×${height}`}
          </p>
        </div>
        <div className="text-right" aria-label={`Total length ${clock(total)}`}>
          <span className="score led block text-5xl leading-none font-bold tabular-nums sm:text-6xl">{clock(total)}</span>
          <span className="text-xs text-white/50">total length</span>
        </div>
      </div>

      <div className="relative flex h-20 gap-0.5 overflow-hidden rounded-md bg-white/5 sm:h-24" aria-hidden>
        {items.length === 0 && (
          <div className="grid flex-1 place-items-center rounded-md border border-dashed border-white/20 text-sm text-white/40">
            Your videos line up here
          </div>
        )}
        {items.map((it, i) => (
          <div
            key={it.id}
            className={cn("relative min-w-7 overflow-hidden bg-white/10 bg-cover bg-center transition-[flex-grow] duration-300",
              it.missing && "opacity-40 grayscale")}
            style={{ flexGrow: Math.max(it.media.duration, 0.1), flexBasis: 0, backgroundImage: it.missing ? undefined : `url(${api.itemThumbUrl(it.id)})` }}
          >
            <span className="score absolute bottom-1 left-1.5 text-lg font-bold drop-shadow-[0_1px_2px_rgb(0_0_0/0.9)]">{i + 1}</span>
          </div>
        ))}
        {(running || job.status === "done") && items.length > 0 && (
          <>
            <div className="absolute inset-y-0 right-0 bg-board/70 transition-[left] duration-500" style={{ left: `${progress * 100}%` }} />
            {running && (
              <div className="absolute inset-y-0 w-0.5 bg-court shadow-[0_0_12px_2px] shadow-court transition-[left] duration-500"
                style={{ left: `${progress * 100}%` }} />
            )}
          </>
        )}
      </div>
    </section>
  );
}
