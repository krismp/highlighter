import { useState, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/**
 * A button that asks inline before acting: the first click swaps it for the
 * question and Confirm / Cancel. (Native confirm() dialogs are not reliable
 * inside desktop webviews.)
 */
export function ConfirmButton({ children, question, confirmLabel, onConfirm, disabled, title, className }: {
  children: ReactNode;
  question: string;
  confirmLabel: string;
  onConfirm: () => void;
  disabled?: boolean;
  title?: string;
  className?: string;
}) {
  const [asking, setAsking] = useState(false);
  if (!asking) {
    return (
      <Button size="sm" variant="ghost" className={className} disabled={disabled} title={title}
        onClick={(e) => { e.preventDefault(); setAsking(true); }}>
        {children}
      </Button>
    );
  }
  return (
    <span className={cn("inline-flex flex-wrap items-center gap-1 text-sm", className)} role="group" aria-label={question}>
      <span className="text-muted-foreground">{question}</span>
      <Button size="sm" variant="destructive" disabled={disabled}
        onClick={(e) => { e.preventDefault(); setAsking(false); onConfirm(); }}>
        {confirmLabel}
      </Button>
      <Button size="sm" variant="ghost" onClick={(e) => { e.preventDefault(); setAsking(false); }}>Cancel</Button>
    </span>
  );
}
