import { RotateCw, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { CopyButton } from "@/components/ErrorAlerts";
import { Button } from "@/components/ui/button";
import { ApiError } from "@/lib/api";
import { formatReport } from "@/lib/errors";

/** In-page error for when a page cannot show anything else (its data failed to load, or it crashed). */
export function ErrorPanel({ title, error, onRetry, retryLabel = "Try again" }:
  { title: string; error: unknown; onRetry?: () => void; retryLabel?: string }) {
  const [time] = useState(() => new Date());
  const message = error instanceof Error ? error.message || error.name : String(error);
  const detail = error instanceof ApiError ? error.request : error instanceof Error ? (error.stack ?? "") : "";
  return (
    <div role="alert" className="mx-auto grid max-w-xl gap-3 rounded-lg border border-destructive/40 bg-card p-6">
      <div className="flex items-center gap-2">
        <TriangleAlert className="size-5 text-destructive" aria-hidden />
        <h1 className="font-heading text-xl font-bold">{title}</h1>
      </div>
      <p className="break-words whitespace-pre-wrap select-text">{message}</p>
      {detail && (
        <details className="text-xs text-muted-foreground">
          <summary className="cursor-pointer">Technical details</summary>
          <pre className="mt-1 max-h-48 overflow-auto font-mono break-all whitespace-pre-wrap select-text">{detail}</pre>
        </details>
      )}
      <div className="flex flex-wrap gap-2">
        {onRetry && <Button onClick={onRetry}><RotateCw className="size-4" /> {retryLabel}</Button>}
        <CopyButton text={() => formatReport({ title, message, detail, time })} />
      </div>
    </div>
  );
}
