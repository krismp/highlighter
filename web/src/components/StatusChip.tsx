import { Check, LoaderCircle } from "lucide-react";
import type { VideoStatus } from "@/lib/api";

/** A video's state in a word: spinning while processing, an orange dot for a draft, a tick when done. */
export function StatusChip({ status }: { status: VideoStatus }) {
  if (status === "empty") return null;
  return (
    <span className="inline-flex items-center gap-1 text-xs">
      {status === "processing" && <LoaderCircle className="size-3.5 animate-spin" aria-hidden />}
      {status === "draft" && <span className="size-2 rounded-full bg-court" aria-hidden />}
      {status === "done" && <Check className="size-3.5 text-net" aria-hidden />}
      {status}
    </span>
  );
}
