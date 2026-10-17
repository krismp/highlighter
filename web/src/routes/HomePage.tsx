import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { OpenVideoForm } from "@/components/OpenVideoForm";
import { SettingsCard } from "@/components/SettingsCard";
import { ToolsBanner } from "@/components/ToolsBanner";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { api } from "@/lib/api";

export function HomePage() {
  const recent = useQuery({ queryKey: ["recent"], queryFn: api.recent });
  return (
    <div className="grid gap-6">
      <ToolsBanner />
      <Card>
        <CardHeader><CardTitle>Clip a match</CardTitle></CardHeader>
        <CardContent>
          <OpenVideoForm autoFocus />
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
