import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { api, type LineError, type SessionView } from "@/lib/api";
import { reportError } from "@/lib/errors";

export function AddClips({ videoId, initialDraft, onAdded }: { videoId: string; initialDraft: string; onAdded: (s: SessionView) => void }) {
  const qc = useQueryClient();
  const [text, setText] = useState(initialDraft);
  // The draft save waits for a pause in typing; `pending` holds the text it will send.
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pending = useRef<string | null>(null);
  const sendDraft = (t: string) => api.saveDraft(videoId, t).then(() => qc.invalidateQueries({ queryKey: ["recent"] }), (err) => reportError("Couldn't save your draft", err));
  const cancelDraft = () => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
    pending.current = null;
  };
  const scheduleDraft = (t: string) => {
    // Keep the cached session's draft current so a remount (before or after the PUT) starts from this text.
    qc.setQueryData<SessionView>(["session", videoId], (s) => s && { ...s, draft: t });
    if (timer.current) clearTimeout(timer.current);
    pending.current = t;
    timer.current = setTimeout(() => {
      timer.current = null;
      pending.current = null;
      void sendDraft(t);
    }, 500);
  };
  // Leaving the page with a save still waiting: send it now rather than lose it.
  const flush = useRef<() => void>(() => {});
  useEffect(() => {
    flush.current = () => {
      const t = pending.current;
      cancelDraft();
      if (t !== null) void sendDraft(t);
    };
  });
  useEffect(() => () => flush.current(), []);
  const [errors, setErrors] = useState<LineError[]>([]);
  const add = useMutation({
    meta: { errorTitle: "Couldn't add the clips" },
    scope: { id: `session-${videoId}` },
    mutationFn: () => {
      cancelDraft(); // the server stores the failed lines as the draft itself
      return api.parseClips(videoId, text);
    },
    onSuccess: (r) => {
      cancelDraft(); // typing while the request ran must not overwrite those lines
      onAdded(r.session);
      setErrors(r.errors);
      setText(r.errors.map((e) => e.text).join("\n")); // keep only the lines that need fixing
    },
  });
  return (
    <div className="grid gap-2">
      <Textarea
        rows={5}
        placeholder={"Paste start times or ranges, one per line:\n12:34 Andi crossover\n2:22 - 3:02 full possession\n1:02:03 buzzer beater"}
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          scheduleDraft(e.target.value);
        }}
        className="font-mono text-sm"
      />
      <Button disabled={!text.trim() || add.isPending} onClick={() => add.mutate()}>Add clips</Button>
      {errors.map((e) => (
        <p key={e.line} className="text-xs text-destructive">Line {e.line}: {e.err}</p>
      ))}
    </div>
  );
}
