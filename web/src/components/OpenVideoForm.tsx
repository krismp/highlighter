import { useMutation } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";

/** Paste a YouTube URL and go straight to that video's clip page. */
export function OpenVideoForm({ autoFocus = false }: { autoFocus?: boolean }) {
  const navigate = useNavigate();
  const [url, setUrl] = useState("");
  const open = useMutation({
    mutationFn: () => api.openVideo(url),
    onSuccess: (s) => navigate({ to: "/s/$videoId", params: { videoId: s.videoId } }),
  });
  return (
    <div>
      <form className="flex flex-col gap-2 sm:flex-row" onSubmit={(e) => { e.preventDefault(); open.mutate(); }}>
        <Input autoFocus={autoFocus} aria-label="YouTube video URL" placeholder="https://www.youtube.com/watch?v=…"
          value={url} onChange={(e) => setUrl(e.target.value)} />
        <Button type="submit" disabled={!url.trim() || open.isPending}>{open.isPending ? "Loading…" : "Open"}</Button>
      </form>
      {open.error && <p className="mt-2 text-sm text-destructive">{open.error.message}</p>}
    </div>
  );
}
