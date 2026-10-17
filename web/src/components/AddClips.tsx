import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { api, type LineError, type SessionView } from "@/lib/api";

export function AddClips({ videoId, onAdded }: { videoId: string; onAdded: (s: SessionView) => void }) {
  const [text, setText] = useState("");
  const [errors, setErrors] = useState<LineError[]>([]);
  const add = useMutation({
    scope: { id: `session-${videoId}` },
    mutationFn: () => api.parseClips(videoId, text),
    onSuccess: (r) => {
      onAdded(r.session);
      setErrors(r.errors);
      setText(r.errors.map((e) => e.text).join("\n")); // keep only the lines that need fixing
    },
  });
  return (
    <div className="grid gap-2">
      <Textarea
        rows={5}
        placeholder={"Paste start times, one per line:\n12:34 Andi crossover\n15:02\n1:02:03 buzzer beater"}
        value={text}
        onChange={(e) => setText(e.target.value)}
        className="font-mono text-sm"
      />
      <Button disabled={!text.trim() || add.isPending} onClick={() => add.mutate()}>Add clips</Button>
      {errors.map((e) => (
        <p key={e.line} className="text-xs text-destructive">Line {e.line}: {e.err}</p>
      ))}
      {add.error && <p className="text-xs text-destructive">{add.error.message}</p>}
    </div>
  );
}
