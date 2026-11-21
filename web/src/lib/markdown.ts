/**
 * A small Markdown reader for release notes: headings, paragraphs, bullet and
 * numbered lists, code blocks, **bold**, *italic*, `code` and [links](url).
 * It returns plain data; the Markdown component turns it into React elements,
 * so no HTML from the notes is ever injected into the page.
 */

export type Inline =
  | { type: "text"; text: string }
  | { type: "strong"; children: Inline[] }
  | { type: "em"; children: Inline[] }
  | { type: "code"; text: string }
  | { type: "link"; href: string; children: Inline[] };

export type Block =
  | { type: "heading"; level: number; children: Inline[] }
  | { type: "paragraph"; children: Inline[] }
  | { type: "list"; ordered: boolean; items: Inline[][] }
  | { type: "code"; text: string };

const heading = /^(#{1,6})\s+(.*?)\s*#*\s*$/;
const bullet = /^\s*[-*+]\s+(.*)$/;
const numbered = /^\s*\d+[.)]\s+(.*)$/;
const fence = /^\s*(```|~~~)/;

/** Parses Markdown into blocks. Lines that continue a list item are joined to it. */
export function parseMarkdown(src: string): Block[] {
  const lines = src.replace(/\r\n?/g, "\n").split("\n");
  const blocks: Block[] = [];
  let para: string[] = [];
  let list: { ordered: boolean; items: string[] } | null = null;

  const flush = () => {
    if (para.length) blocks.push({ type: "paragraph", children: parseInline(para.join(" ")) });
    if (list) blocks.push({ type: "list", ordered: list.ordered, items: list.items.map(parseInline) });
    para = [];
    list = null;
  };

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const open = fence.exec(line);
    if (open) {
      flush();
      const code: string[] = [];
      while (++i < lines.length && !lines[i].trimStart().startsWith(open[1])) code.push(lines[i]);
      blocks.push({ type: "code", text: code.join("\n") });
      continue;
    }
    if (line.trim() === "") {
      flush();
      continue;
    }
    const h = heading.exec(line);
    if (h) {
      flush();
      blocks.push({ type: "heading", level: h[1].length, children: parseInline(h[2]) });
      continue;
    }
    const b = bullet.exec(line);
    const n = b ? null : numbered.exec(line);
    if (b || n) {
      const ordered = n !== null;
      if (para.length || (list && list.ordered !== ordered)) flush();
      list ??= { ordered, items: [] };
      list.items.push((b ?? n)![1]);
      continue;
    }
    if (list) list.items[list.items.length - 1] += " " + line.trim();
    else para.push(line.trim());
  }
  flush();
  return blocks;
}

/** Parses **bold**, *italic* / _italic_, `code` and [text](url) within one block. */
export function parseInline(src: string): Inline[] {
  const out: Inline[] = [];
  let text = "";
  const pushText = () => {
    if (text) out.push({ type: "text", text });
    text = "";
  };
  let i = 0;
  while (i < src.length) {
    const rest = src.slice(i);
    let m: RegExpExecArray | null;
    if ((m = /^`([^`]+)`/.exec(rest))) {
      pushText();
      out.push({ type: "code", text: m[1] });
    } else if ((m = /^\*\*(.+?)\*\*/.exec(rest)) || (m = /^__(.+?)__/.exec(rest))) {
      pushText();
      out.push({ type: "strong", children: parseInline(m[1]) });
    } else if ((m = /^\*(?!\s)(.+?)\*/.exec(rest)) || (m = /^_(?!\s)(.+?)_(?![A-Za-z0-9])/.exec(rest))) {
      pushText();
      out.push({ type: "em", children: parseInline(m[1]) });
    } else if ((m = /^\[([^\]]+)\]\(([^)\s]+)\)/.exec(rest))) {
      pushText();
      out.push({ type: "link", href: m[2], children: parseInline(m[1]) });
    } else {
      text += src[i];
      i++;
      continue;
    }
    i += m[0].length;
  }
  pushText();
  return out;
}

/**
 * The part of the release notes worth showing in the app: everything before
 * the standard download and install instructions.
 */
export function whatsNew(notes: string): string {
  const cut = notes.search(/^## Download\b/m);
  return (cut < 0 ? notes : notes.slice(0, cut)).trim();
}
