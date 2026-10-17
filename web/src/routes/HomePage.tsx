import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { SettingsCard } from "@/components/SettingsCard";
import { ToolsBanner } from "@/components/ToolsBanner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";

export function HomePage() {
  const navigate = useNavigate();
  const [url, setUrl] = useState("");
  const recent = useQuery({ queryKey: ["recent"], queryFn: api.recent });
  const open = useMutation({
    mutationFn: () => api.openVideo(url),
    onSuccess: (s) => navigate({ to: "/s/$videoId", params: { videoId: s.videoId } }),
  });
  return (
    <div className="grid gap-6">
      <ToolsBanner />
      <Card>
        <CardHeader><CardTitle>Clip a match</CardTitle></CardHeader>
        <CardContent>
          <form className="flex flex-col gap-2 sm:flex-row" onSubmit={(e) => { e.preventDefault(); open.mutate(); }}>
            <Input autoFocus placeholder="https://www.youtube.com/watch?v=…" value={url} onChange={(e) => setUrl(e.target.value)} />
            <Button type="submit" disabled={!url.trim() || open.isPending}>{open.isPending ? "Loading…" : "Open"}</Button>
          </form>
          {open.error && <p className="mt-2 text-sm text-destructive">{open.error.message}</p>}
        </CardContent>
      </Card>
      {recent.data && recent.data.length > 0 && (
        <Card>
          <CardHeader><CardTitle>Recent videos</CardTitle></CardHeader>
          <CardContent>
            <ul className="divide-y">
              {recent.data.map((r) => (
                <li key={r.videoId}>
                  <Link to="/s/$videoId" params={{ videoId: r.videoId }} className="flex justify-between gap-4 py-2 hover:underline">
                    <span className="truncate">{r.title}</span>
                    <span className="shrink-0 text-sm text-muted-foreground">
                      {r.clipCount} clips · {new Date(r.updatedAt).toLocaleDateString()}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}
      <SettingsCard />
    </div>
  );
}
