import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";

const healthQuery = { queryKey: ["health"], queryFn: api.health, meta: { errorTitle: "Couldn't check the bundled tools" } };

/** Home page warning, shown only when a bundled tool is missing. */
export function ToolsBanner() {
  const health = useQuery(healthQuery);
  if (!health.data) return null;
  const { ytDlp, ffmpeg } = health.data;
  if (ytDlp.found && ffmpeg.found) return null;
  return (
    <div role="alert" className="flex items-start gap-2 rounded-lg border border-destructive/40 bg-card p-3 text-sm">
      <TriangleAlert className="mt-0.5 size-4 shrink-0 text-destructive" aria-hidden />
      <p>
        {!ytDlp.found && !ffmpeg.found ? "yt-dlp and ffmpeg were" : !ytDlp.found ? "yt-dlp was" : "ffmpeg was"} not found.
        They ship with highlighter, so download and reinstall the app, then open it again.
      </p>
    </div>
  );
}

/** Settings page: bundled tool versions and the yt-dlp updater. */
export function ToolsStatus() {
  const qc = useQueryClient();
  const health = useQuery(healthQuery);
  const update = useMutation({ meta: { errorTitle: "Couldn't update yt-dlp" }, mutationFn: api.updateYtDlp, onSettled: () => qc.invalidateQueries({ queryKey: ["health"] }) });
  const h = health.data;
  if (!h) return <p className="text-sm text-muted-foreground">Checking the bundled tools…</p>;
  return (
    <div className="grid gap-3 text-sm">
      <dl className="grid grid-cols-[8rem_1fr] gap-x-4 gap-y-1">
        <dt className="text-muted-foreground">highlighter</dt><dd>{h.version}</dd>
        <dt className="text-muted-foreground">yt-dlp</dt><dd>{h.ytDlp.found ? h.ytDlp.version : "not found"}</dd>
        <dt className="text-muted-foreground">ffmpeg</dt><dd className="truncate">{h.ffmpeg.found ? h.ffmpeg.version.replace(/ Copyright.*/, "") : "not found"}</dd>
        <dt className="text-muted-foreground">deno</dt><dd>{h.deno.found ? h.deno.version : "not found"}</dd>
        <dt className="text-muted-foreground">Video encoder</dt><dd>{h.encoder}</dd>
      </dl>
      <div className="flex flex-wrap items-center gap-3">
        <Button size="sm" variant="outline" disabled={update.isPending} onClick={() => update.mutate()}>
          {update.isPending ? "Updating…" : "Update yt-dlp"}
        </Button>
        <span className="text-muted-foreground">
          {update.data ? update.data.output.trim() || "Up to date" : "If videos stop opening, YouTube may have changed: update yt-dlp."}
        </span>
      </div>
    </div>
  );
}
