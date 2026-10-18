import { useMutation } from "@tanstack/react-query";
import { Power } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";

const headerButton = "text-white/70 hover:bg-white/10 hover:text-white";

/**
 * Stops the local server, which also closes the desktop app. Confirms inline:
 * native confirm() dialogs are not reliable inside desktop webviews.
 */
export function QuitButton() {
  const [asking, setAsking] = useState(false);
  const quit = useMutation({ meta: { errorTitle: "Couldn't quit" }, mutationFn: api.quit });
  if (quit.isSuccess) {
    return (
      <div className="fixed inset-0 z-50 grid place-items-center bg-background p-6 text-center text-foreground">
        <div className="grid gap-2">
          <p className="font-heading text-2xl font-bold">highlighter has stopped</p>
          <p className="text-muted-foreground">You can close this window. Open the app again whenever you need it.</p>
        </div>
      </div>
    );
  }
  if (!asking) {
    return (
      <Button size="sm" variant="ghost" className={headerButton} title="Stop highlighter" onClick={() => setAsking(true)}>
        <Power className="size-3.5" /> Quit
      </Button>
    );
  }
  return (
    <div className="flex items-center gap-1 text-sm">
      <span className="hidden text-white/70 sm:inline">
        Quit? Clips still being cut will stop.
      </span>
      <Button size="sm" variant="ghost" className={headerButton} disabled={quit.isPending} onClick={() => quit.mutate()}>
        Quit
      </Button>
      <Button size="sm" variant="ghost" className={headerButton} disabled={quit.isPending} onClick={() => setAsking(false)}>
        Cancel
      </Button>
    </div>
  );
}
