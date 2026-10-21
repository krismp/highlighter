import { useMutation } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { LoaderCircle } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useConnectivity } from "@/hooks/useConnectivity";
import { api } from "@/lib/api";

/** Paste a YouTube URL and go straight to that video's clip page. */
export function OpenVideoForm({ autoFocus = false, size = "default" }: { autoFocus?: boolean; size?: "default" | "lg" }) {
  const navigate = useNavigate();
  const [url, setUrl] = useState("");
  const net = useConnectivity();
  const open = useMutation({
    meta: { errorTitle: "Couldn't open the video" },
    mutationFn: () => api.openVideo(url),
    onSuccess: (s) => navigate({ to: "/s/$videoId", params: { videoId: s.videoId } }),
  });
  return (
    <div>
      <form className={size === "lg" ? "flex flex-col gap-2 sm:flex-row sm:gap-0 sm:rounded-full sm:border sm:bg-card sm:p-1.5 sm:shadow-md sm:focus-within:ring-3 sm:focus-within:ring-ring/50" : "flex flex-col gap-2 sm:flex-row"} onSubmit={(e) => { e.preventDefault(); open.mutate(); }}>
        <Input autoFocus={autoFocus} aria-label="YouTube video URL" placeholder="https://www.youtube.com/watch?v=…"
          className={size === "lg" ? "h-11 rounded-full px-5 text-base sm:border-0 sm:shadow-none sm:focus-visible:ring-0 md:text-base" : undefined}
          value={url} onChange={(e) => setUrl(e.target.value)} />
        <Button type="submit" size={size === "lg" ? "lg" : "default"} className={size === "lg" ? "h-11 rounded-full px-6" : undefined} disabled={!url.trim() || open.isPending || net.online === false}>{open.isPending ? <><LoaderCircle className="size-4 animate-spin" /> Opening…</> : "Open"}</Button>
      </form>
      {net.online === false && <p className="mt-2 text-sm text-muted-foreground">Opening a video needs an internet connection.</p>}
    </div>
  );
}
