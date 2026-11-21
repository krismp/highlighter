import { Fragment } from "react";
import { parseMarkdown, type Block, type Inline } from "@/lib/markdown";
import { cn } from "@/lib/utils";

/**
 * Renders release-notes Markdown as React elements (never raw HTML). Links show
 * as text: the desktop app can't open a browser, and following one would
 * leave the app.
 */
export function Markdown({ text, className }: { text: string; className?: string }) {
  return <div className={cn("grid gap-2 leading-relaxed", className)}>{parseMarkdown(text).map((b, i) => <BlockView key={i} block={b} />)}</div>;
}

function BlockView({ block }: { block: Block }) {
  switch (block.type) {
    case "heading":
      return block.level <= 2
        ? <h3 className="font-heading text-lg font-bold">{inlines(block.children)}</h3>
        : <h4 className="mt-1 font-heading text-base font-semibold">{inlines(block.children)}</h4>;
    case "paragraph":
      return <p>{inlines(block.children)}</p>;
    case "list": {
      const Tag = block.ordered ? "ol" : "ul";
      return (
        <Tag className={cn("grid gap-1 pl-5", block.ordered ? "list-decimal" : "list-disc")}>
          {block.items.map((item, i) => <li key={i}>{inlines(item)}</li>)}
        </Tag>
      );
    }
    case "code":
      return <pre className="overflow-x-auto rounded-md bg-background px-3 py-2 font-mono text-xs">{block.text}</pre>;
  }
}

function inlines(nodes: Inline[]) {
  return nodes.map((n, i) => <Fragment key={i}>{inline(n)}</Fragment>);
}

function inline(n: Inline) {
  switch (n.type) {
    case "text":
      return n.text;
    case "strong":
      return <strong className="font-semibold">{inlines(n.children)}</strong>;
    case "em":
      return <em>{inlines(n.children)}</em>;
    case "code":
      return <code className="rounded bg-background px-1 py-0.5 font-mono text-[0.85em]">{n.text}</code>;
    case "link":
      return <span className="underline decoration-dotted" title={n.href}>{inlines(n.children)}</span>;
  }
}
