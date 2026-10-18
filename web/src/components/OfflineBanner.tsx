import { RotateCw, WifiOff } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useConnectivity } from "@/hooks/useConnectivity";

/** Explains, on every page, that nothing that touches YouTube works until the internet is back. */
export function OfflineBanner() {
  const net = useConnectivity();
  if (net.online !== false) return null;
  return (
    <div role="status" className="border-b border-destructive/30 bg-destructive/10">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3 text-sm">
        <WifiOff className="size-5 shrink-0 text-destructive" aria-hidden />
        <div className="min-w-0 flex-1">
          <p className="font-semibold">No internet connection</p>
          <p className="text-muted-foreground">
            Highlighter needs the internet to open YouTube videos, play them and cut clips. Your clips and saved
            files are safe; this page checks again automatically.
          </p>
          {net.error && <p className="mt-0.5 text-xs break-words text-muted-foreground select-text">{net.error}</p>}
        </div>
        <Button size="sm" variant="outline" disabled={net.checking} onClick={() => net.recheck()}>
          <RotateCw className={net.checking ? "size-3.5 animate-spin" : "size-3.5"} /> {net.checking ? "Checking…" : "Check again"}
        </Button>
      </div>
    </div>
  );
}
