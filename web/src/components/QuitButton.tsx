import { useMutation } from "@tanstack/react-query";
import { Power } from "lucide-react";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";

/** Stops the local server. The macOS app runs it in the background, so this is how it ends. */
export function QuitButton() {
  const quit = useMutation({ mutationFn: api.quit });
  if (quit.isSuccess) {
    return (
      <div className="fixed inset-0 z-50 grid place-items-center bg-background p-6 text-center text-foreground">
        <div className="grid gap-2">
          <p className="font-heading text-2xl font-bold">highlighter has stopped</p>
          <p className="text-muted-foreground">You can close this tab. Open the app again whenever you need it.</p>
        </div>
      </div>
    );
  }
  return (
    <Button
      size="sm"
      variant="ghost"
      className="ml-auto text-white/70 hover:bg-white/10 hover:text-white"
      disabled={quit.isPending}
      title={quit.error ? quit.error.message : "Stop highlighter"}
      onClick={() => {
        if (window.confirm("Quit highlighter? Clips that are still being cut will stop.")) quit.mutate();
      }}
    >
      <Power className="size-3.5" /> Quit
    </Button>
  );
}
