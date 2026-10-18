import { Check, Copy, TriangleAlert, X } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { copyText, dismissError, formatReport, reportError, useReportedErrors, type ReportedError } from "@/lib/errors";

/** Every reported error, as closable alerts in the corner, each with its report one click away. */
export function ErrorAlerts() {
  const errors = useReportedErrors();
  if (errors.length === 0) return null;
  return (
    <div className="fixed right-4 bottom-4 z-40 grid w-[min(28rem,calc(100vw-2rem))] gap-2" role="region" aria-label="Errors">
      {errors.map((e) => <ErrorAlert key={e.id} error={e} />)}
    </div>
  );
}

function ErrorAlert({ error }: { error: ReportedError }) {
  return (
    <div role="alert" className="rounded-lg border border-destructive/40 bg-card p-3 text-sm shadow-lg">
      <div className="flex items-start gap-2">
        <TriangleAlert className="mt-0.5 size-4 shrink-0 text-destructive" aria-hidden />
        <div className="min-w-0 flex-1">
          <p className="font-semibold">
            {error.title}
            {error.count > 1 && <span className="ml-1.5 font-normal text-muted-foreground">×{error.count}</span>}
          </p>
          <p className="mt-0.5 break-words whitespace-pre-wrap select-text">{error.message}</p>
          {error.detail && (
            <details className="mt-1 text-xs text-muted-foreground">
              <summary className="cursor-pointer">Technical details</summary>
              <pre className="mt-1 max-h-40 overflow-auto font-mono break-all whitespace-pre-wrap select-text">{error.detail}</pre>
            </details>
          )}
          <CopyButton text={() => formatReport(error)} className="mt-2" />
        </div>
        <Button size="icon" variant="ghost" className="size-7 shrink-0" aria-label="Close" onClick={() => dismissError(error.id)}>
          <X className="size-4" />
        </Button>
      </div>
    </div>
  );
}

/** Copies an error report for pasting into a bug report. */
export function CopyButton({ text, className }: { text: () => string; className?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <Button size="sm" variant="outline" className={className}
      onClick={() => copyText(text()).then(() => {
        setCopied(true);
        window.setTimeout(() => setCopied(false), 2000);
      }, (err) => reportError("Couldn't copy the error details", err))}>
      {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />} {copied ? "Copied" : "Copy details"}
    </Button>
  );
}
