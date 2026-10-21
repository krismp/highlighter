import { useQuery } from "@tanstack/react-query";
import { Check, Plus } from "lucide-react";
import { api, type CombineItem } from "@/lib/api";
import { cn } from "@/lib/utils";

/** The newest videos in the clips folder, one click to add each. */
export function Library({ items, disabled, onAdd }: { items: CombineItem[]; disabled: boolean; onAdd: (path: string) => void }) {
  const lib = useQuery({ queryKey: ["library"], queryFn: api.library, meta: { errorTitle: "Couldn't list your highlights" } });
  const files = lib.data?.files ?? [];
  const added = new Map<string, number>();
  for (const it of items) added.set(it.path, (added.get(it.path) ?? 0) + 1);

  return (
    <section aria-labelledby="library-heading" className="grid gap-2">
      <div className="px-1">
        <h2 id="library-heading" className="font-heading text-2xl font-bold">From your highlights</h2>
        <p className="text-sm text-muted-foreground">
          {lib.isPending ? "Looking in your clips folder…"
            : files.length === 0 ? <>No videos in <span className="break-all">{lib.data?.root ?? "your clips folder"}</span> yet. Clips you cut show up here.</>
            : "Newest first. Click one to add it to the end."}
        </p>
      </div>
      {files.length > 0 && (
        <ul className="grid grid-cols-[repeat(auto-fill,minmax(10rem,1fr))] gap-2">
          {files.map((f) => {
            const count = added.get(f.path) ?? 0;
            return (
              <li key={f.path}>
                <button type="button" disabled={disabled} onClick={() => onAdd(f.path)}
                  title={count ? `Added${count > 1 ? ` ${count} times` : ""}. Click to add it again.` : `Add ${f.name}`}
                  className={cn("group grid w-full gap-1 rounded-lg border bg-card p-1.5 text-left transition-colors hover:border-court focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:opacity-50",
                    count > 0 && "border-court/60 bg-accent/50")}>
                  <span className="relative block aspect-video overflow-hidden rounded-md bg-board">
                    <img src={api.libraryThumbUrl(f.path)} alt="" loading="lazy" className="size-full object-contain" />
                    <span className={cn("absolute top-1 right-1 grid size-6 place-items-center rounded-full shadow transition-transform",
                      count > 0 ? "bg-court text-white" : "scale-90 bg-white/90 text-board opacity-0 group-hover:scale-100 group-hover:opacity-100 group-focus-visible:opacity-100")}>
                      {count > 0 ? <Check className="size-3.5" /> : <Plus className="size-3.5" />}
                    </span>
                  </span>
                  <span className="truncate px-0.5 text-sm font-medium">{f.name}</span>
                  <span className="-mt-1 truncate px-0.5 text-xs text-muted-foreground">{f.folder}</span>
                </button>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
