import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";

export function ToolsBanner() {
  const qc = useQueryClient();
  const health = useQuery({ queryKey: ["health"], queryFn: api.health });
  const update = useMutation({ mutationFn: api.updateYtDlp, onSettled: () => qc.invalidateQueries({ queryKey: ["health"] }) });
  if (!health.data) return null;
  const { ytDlp, ffmpeg, encoder } = health.data;
  const missing = !ytDlp.found || !ffmpeg.found;
  return (
    <div className={missing ? "rounded-md border border-destructive p-3 text-sm" : "rounded-md border p-3 text-sm text-muted-foreground"}>
      {missing ? (
        <p>
          {!ytDlp.found && !ffmpeg.found ? "yt-dlp and ffmpeg were" : !ytDlp.found ? "yt-dlp was" : "ffmpeg was"} not found.
          They ship with highlighter, so download and reinstall the app, then open it again.
        </p>
      ) : (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <span>yt-dlp {ytDlp.version}</span>
          <span>encoder {encoder}</span>
          <Button size="sm" variant="outline" disabled={update.isPending} onClick={() => update.mutate()}>
            {update.isPending ? "Updating…" : "Update yt-dlp"}
          </Button>
          {update.data && <span>{update.data.output.trim() || "Up to date"}</span>}
          {update.error && <span className="text-destructive">{update.error.message}</span>}
        </div>
      )}
    </div>
  );
}
