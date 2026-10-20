import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowDownToLine, CircleCheck, RotateCw, Sparkles, X } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { api } from "@/lib/api";
import { inDesktopApp } from "@/lib/desktop";
import { bytes } from "@/lib/format";
import { useUpdateCheck } from "@/hooks/useUpdateCheck";
import { dismissedVersion, dismissVersion, installUpdate, useInstallState, type InstallState } from "@/lib/updates";
import { cn } from "@/lib/utils";

/**
 * Updating restarts the app, which stops anything still processing: ask
 * first in that case, otherwise install right away.
 */
function UpdateButton({ dark }: { dark?: boolean }) {
  const install = useInstallState();
  const [running, setRunning] = useState<string[] | null>(null);
  const busy = install.phase === "downloading" || install.phase === "installing";
  const start = async () => {
    const titles = await api.runningJobs().then((r) => r.running.map((j) => j.title), () => []);
    if (titles.length > 0) setRunning(titles);
    else installUpdate();
  };
  if (running) {
    return (
      <span className="inline-flex flex-wrap items-center gap-1.5 text-sm" role="group" aria-label="Confirm update">
        <span>Updating restarts highlighter and stops {running.length === 1 ? `"${running[0]}"` : `${running.length} videos being processed`}.</span>
        <Button size="sm" variant="destructive" onClick={() => { setRunning(null); installUpdate(); }}>Update anyway</Button>
        <Button size="sm" variant="ghost" className={cn(dark && "text-white hover:bg-white/10 hover:text-white")} onClick={() => setRunning(null)}>Wait</Button>
      </span>
    );
  }
  return (
    <Button size="sm" disabled={busy} onClick={start}>
      {install.phase === "failed" ? <RotateCw /> : <ArrowDownToLine />}
      {install.phase === "failed" ? "Try again" : "Update and restart"}
    </Button>
  );
}

function Progress({ install, dark }: { install: InstallState; dark?: boolean }) {
  if (install.phase !== "downloading" && install.phase !== "installing") return null;
  const pct = install.phase === "installing" ? 100 : install.total ? (install.downloaded / install.total) * 100 : 0;
  return (
    <div className="grid min-w-48 flex-1 gap-1" aria-live="polite">
      <span className={cn("text-xs", dark ? "text-white/70" : "text-muted-foreground")}>
        {install.phase === "installing" ? "Installing, highlighter restarts in a moment…"
          : `Downloading${install.total ? ` ${bytes(install.downloaded)} of ${bytes(install.total)}` : "…"}`}
      </span>
      <div className={cn("h-1.5 overflow-hidden rounded-full", dark ? "bg-white/15" : "bg-muted")} role="progressbar" aria-label="Update download"
        aria-valuenow={Math.round(pct)} aria-valuemin={0} aria-valuemax={100}>
        <div className="progress-stripes h-full rounded-full bg-court transition-[width] duration-300" style={{ width: `${Math.max(4, pct)}%` }} />
      </div>
    </div>
  );
}

/** A slim bar under the header when a new version is out. "Later" hides it until the next version. */
export function UpdateBanner() {
  const update = useUpdateCheck();
  const install = useInstallState();
  const [hidden, setHidden] = useState(dismissedVersion);
  const u = update.data;
  if (!u || (hidden === u.version && install.phase === "idle")) return null;
  return (
    <div role="status" className="border-b border-white/10 bg-board text-white">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-3 gap-y-2 px-4 py-2 text-sm">
        <Sparkles className="size-4 shrink-0 text-led" aria-hidden />
        <p className="min-w-0">
          <span className="font-semibold">Highlighter {u.version} is available.</span>{" "}
          <span className="text-white/60">You have {u.currentVersion}. It installs over this one; your clips and settings stay.</span>
        </p>
        <Progress install={install} dark />
        {install.phase === "failed" && <span className="text-xs text-red-300">Update failed: {install.error}</span>}
        <div className="ml-auto flex items-center gap-1">
          <Link to="/settings" hash="updates" className="rounded-md px-2 py-1 text-white/70 hover:bg-white/10 hover:text-white">What's new</Link>
          <UpdateButton dark />
          {install.phase === "idle" && (
            <Button size="icon-sm" variant="ghost" className="text-white/70 hover:bg-white/10 hover:text-white" title="Remind me at the next version"
              onClick={() => { dismissVersion(u.version); setHidden(u.version); }}>
              <X /><span className="sr-only">Later</span>
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

/** Settings: the installed version, a manual check, release notes and the install. */
export function UpdatesCard() {
  const update = useUpdateCheck();
  const install = useInstallState();
  const health = useQuery({ queryKey: ["health"], queryFn: api.health, staleTime: Infinity, meta: { errorTitle: "Couldn't check the bundled tools" } });
  const u = update.data;
  return (
    <Card id="updates">
      <CardHeader><CardTitle>Updates</CardTitle></CardHeader>
      <CardContent className="grid gap-3 text-sm">
        {!inDesktopApp ? (
          <p className="text-muted-foreground">
            The desktop app checks for new versions and installs them over itself. You're running highlighter {health.data?.version ?? ""} from source:
            update it with <code>git pull && make build</code>.
          </p>
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-3">
              {update.isFetching ? <span className="text-muted-foreground">Checking for updates…</span>
                : update.isError ? <span className="text-destructive">Couldn't check for updates: {String(update.error instanceof Error ? update.error.message : update.error)}</span>
                : u ? <span className="font-semibold">Highlighter {u.version} is available. You have {u.currentVersion}.</span>
                : update.isSuccess ? <span className="inline-flex items-center gap-1.5"><CircleCheck className="size-4 text-net" /> You have the latest version{health.data ? ` (${health.data.version})` : ""}.</span>
                : null}
              <Button size="sm" variant="outline" className="ml-auto" disabled={update.isFetching || install.phase !== "idle" && install.phase !== "failed"} onClick={() => update.refetch()}>
                <RotateCw className={update.isFetching ? "animate-spin" : undefined} /> Check for updates
              </Button>
            </div>
            {u && (
              <>
                {u.notes && (
                  <div className="max-h-56 overflow-auto rounded-md border bg-muted/40 p-3 whitespace-pre-wrap select-text">
                    {u.date && <p className="mb-1 text-xs text-muted-foreground">Released {new Date(u.date * 1000).toLocaleDateString()}</p>}
                    {u.notes}
                  </div>
                )}
                <div className="flex flex-wrap items-center gap-3">
                  <UpdateButton />
                  <Progress install={install} />
                  {install.phase === "failed" && <span className="text-destructive">Update failed: {install.error}</span>}
                </div>
                <p className="text-xs text-muted-foreground">
                  The new version is checked against highlighter's signing key, installed over this one, and opened. No uninstalling, and your
                  clips, sessions and settings stay as they are.
                </p>
              </>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
