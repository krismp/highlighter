import { Popover } from "@base-ui/react/popover";
import { Link } from "@tanstack/react-router";
import { Clapperboard, LoaderCircle } from "lucide-react";
import { useState } from "react";
import { StatusChip } from "@/components/StatusChip";
import { useRecent } from "@/hooks/useRecent";
import { unfinished } from "@/lib/api";

/** Header menu: the five most recent videos, unfinished ones first, with a badge counting them. */
export function VideosMenu() {
  const [open, setOpen] = useState(false);
  const recent = useRecent();
  const all = recent.data ?? [];
  const count = all.filter((r) => unfinished(r.status)).length;
  const processing = all.some((r) => r.status === "processing");
  const rows = [...all]
    .sort((a, b) => Number(unfinished(b.status)) - Number(unfinished(a.status)) || b.updatedAt.localeCompare(a.updatedAt));
  return (
    <Popover.Root open={open} onOpenChange={setOpen}>
      <Popover.Trigger
        className="flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-sm text-white/70 hover:bg-white/10 hover:text-white data-[popup-open]:bg-white/10 data-[popup-open]:text-white focus-visible:ring-2 focus-visible:ring-court focus-visible:outline-none"
        title="Your videos"
      >
        <Clapperboard className="size-3.5" /> Videos
        {processing ? (
          <LoaderCircle className="size-3.5 animate-spin" aria-label="Processing" />
        ) : (
          count > 0 && <span className="grid min-w-4 place-items-center rounded-full bg-court px-1 text-[10px] leading-4 font-semibold text-white">{count}</span>
        )}
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Positioner side="bottom" align="end" sideOffset={8} className="z-50">
          <Popover.Popup className="w-80 rounded-lg border bg-card text-card-foreground shadow-lg outline-none">
            {rows.length === 0 ? (
              <p className="p-4 text-sm text-muted-foreground">No videos yet. Paste a YouTube link on Home.</p>
            ) : (
              <ul className="max-h-96 overflow-y-auto p-1">
                {rows.map((r) => (
                  <li key={r.videoId}>
                    <Link
                      to="/s/$videoId"
                      params={{ videoId: r.videoId }}
                      onClick={() => setOpen(false)}
                      className="flex items-center gap-2 rounded-md px-3 py-2 text-sm hover:bg-muted"
                    >
                      <span className="min-w-0 flex-1 truncate">{r.title}</span>
                      <span className="shrink-0 text-xs text-muted-foreground">{r.clipCount} {r.clipCount === 1 ? "clip" : "clips"}</span>
                      <StatusChip status={r.status} />
                    </Link>
                  </li>
                ))}
              </ul>
            )}
            <div className="border-t p-1">
              <Link to="/" hash="recent" onClick={() => setOpen(false)} className="block rounded-md px-3 py-2 text-sm text-muted-foreground hover:bg-muted hover:text-foreground">
                Go to Home
              </Link>
            </div>
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  );
}
