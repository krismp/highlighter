import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";

export function SettingsCard() {
  const qc = useQueryClient();
  const settings = useQuery({ queryKey: ["settings"], queryFn: api.settings, meta: { errorTitle: "Couldn't load settings" } });
  const [outputRoot, setOutputRoot] = useState("");
  const [concurrency, setConcurrency] = useState("3");
  const [defaultDuration, setDefaultDuration] = useState("10");
  useEffect(() => {
    if (!settings.data) return;
    setOutputRoot(settings.data.outputRoot);
    setConcurrency(String(settings.data.concurrency));
    setDefaultDuration(String(settings.data.defaultDuration));
  }, [settings.data]);
  const save = useMutation({
    meta: { errorTitle: "Couldn't save settings" },
    mutationFn: () => api.saveSettings({ outputRoot, concurrency: Number(concurrency), defaultDuration: Number(defaultDuration) }),
    onSuccess: (s) => qc.setQueryData(["settings"], s),
  });
  return (
    <Card>
      <CardHeader><CardTitle>Settings</CardTitle></CardHeader>
      <CardContent>
        <form className="grid gap-3 sm:grid-cols-[1fr_8rem_9rem_auto] sm:items-end" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
          <div className="grid gap-1.5">
            <Label htmlFor="outputRoot">Output folder</Label>
            <Input id="outputRoot" value={outputRoot} onChange={(e) => setOutputRoot(e.target.value)} />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="concurrency">Parallel clips</Label>
            <Input id="concurrency" type="number" min={1} max={8} value={concurrency} onChange={(e) => setConcurrency(e.target.value)} />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="defaultDuration">New clip length (s)</Label>
            <Input id="defaultDuration" type="number" min={1} max={120} step="0.5" value={defaultDuration} onChange={(e) => setDefaultDuration(e.target.value)} />
          </div>
          <Button type="submit" disabled={save.isPending}>Save</Button>
        </form>
        {save.isSuccess && <p className="mt-2 text-sm text-muted-foreground">Saved.</p>}
      </CardContent>
    </Card>
  );
}
