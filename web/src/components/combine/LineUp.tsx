import {
  closestCenter, DndContext, KeyboardSensor, PointerSensor, useSensor, useSensors, type DragEndEvent,
} from "@dnd-kit/core";
import { restrictToParentElement, restrictToVerticalAxis } from "@dnd-kit/modifiers";
import { arrayMove, SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { CircleAlert, GripVertical, Play, Plus, VolumeX, X } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { api, type CombineItem } from "@/lib/api";
import { bytes, length } from "@/lib/format";
import { cn } from "@/lib/utils";

/** The videos in play order. Drag a row by its handle (or focus it, Space, arrows) to move it. */
export function LineUp({ items, disabled, onReorder, onRemove, onAdd, adding }: {
  items: CombineItem[];
  disabled: boolean;
  onReorder: (ids: string[]) => void;
  onRemove: (id: string) => void;
  onAdd: () => void;
  adding: boolean;
}) {
  const [preview, setPreview] = useState<CombineItem | null>(null);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return;
    const ids = items.map((i) => i.id);
    onReorder(arrayMove(ids, ids.indexOf(String(active.id)), ids.indexOf(String(over.id))));
  };
  return (
    <section aria-labelledby="lineup-heading" className="grid content-start gap-2">
      <div className="flex items-baseline justify-between gap-2 px-1">
        <h2 id="lineup-heading" className="font-heading text-2xl font-bold">Play order</h2>
        <p className="text-xs text-muted-foreground">Drag <GripVertical className="inline size-3.5 align-[-2px]" aria-label="the handle" /> to reorder</p>
      </div>
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}
        modifiers={[restrictToVerticalAxis, restrictToParentElement]}
        accessibility={{ screenReaderInstructions: { draggable: "Press Space to pick up a video, the arrow keys to move it, Space again to drop it, or Escape to cancel." } }}>
        <SortableContext items={items.map((i) => i.id)} strategy={verticalListSortingStrategy} disabled={disabled}>
          <ol className="grid gap-1.5">
            {items.map((it, i) => (
              <Row key={it.id} item={it} index={i} disabled={disabled} onRemove={() => onRemove(it.id)} onPreview={() => setPreview(it)} />
            ))}
          </ol>
        </SortableContext>
      </DndContext>
      <button type="button" onClick={onAdd} disabled={disabled || adding}
        className="flex items-center justify-center gap-2 rounded-lg border-2 border-dashed border-input px-3 py-3 text-sm font-medium text-muted-foreground transition-colors hover:border-court hover:bg-accent hover:text-accent-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:pointer-events-none disabled:opacity-50">
        <Plus className="size-4" /> {adding ? "Adding…" : "Add more videos"}
        <span className="hidden font-normal sm:inline">or drop them on the window</span>
      </button>
      {preview && <PreviewDialog item={preview} onClose={() => setPreview(null)} />}
    </section>
  );
}

function Row({ item, index, disabled, onRemove, onPreview }: {
  item: CombineItem; index: number; disabled: boolean; onRemove: () => void; onPreview: () => void;
}) {
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({ id: item.id });
  const m = item.media;
  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Translate.toString(transform), transition }}
      className={cn(
        "group flex items-center gap-2 rounded-lg border bg-card p-1.5 pr-2",
        isDragging && "relative z-10 border-court shadow-xl ring-2 ring-court/30",
        item.missing && "border-destructive/40 bg-destructive/5",
      )}
    >
      <button
        ref={setActivatorNodeRef}
        type="button"
        className="grid h-12 w-6 shrink-0 cursor-grab touch-none place-items-center rounded text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none active:cursor-grabbing disabled:cursor-default disabled:opacity-30"
        aria-label={`Move ${item.name}, video ${index + 1}`}
        disabled={disabled}
        {...attributes}
        {...listeners}
      >
        <GripVertical className="size-4" />
      </button>
      <span className="score w-6 shrink-0 text-center text-xl font-bold text-muted-foreground">{index + 1}</span>
      <button type="button" onClick={onPreview} disabled={item.missing} title="Play this video"
        className="relative aspect-video w-24 shrink-0 overflow-hidden rounded-md bg-board focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none sm:w-28">
        {!item.missing && <img src={api.itemThumbUrl(item.id)} alt="" loading="lazy" className="size-full object-contain" />}
        <span className="absolute inset-0 grid place-items-center bg-board/40 opacity-0 transition-opacity group-hover:opacity-100">
          <Play className="size-5 fill-white text-white" />
        </span>
        <span className="sr-only">Play {item.name}</span>
      </button>
      <div className="min-w-0 flex-1">
        <p className="truncate font-medium" title={item.path}>{item.name}</p>
        <p className="flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
          <span className="truncate">{item.folder}</span>
          {item.missing ? (
            <span className="inline-flex items-center gap-1 font-medium text-destructive"><CircleAlert className="size-3" /> File moved or deleted: remove it or add it again</span>
          ) : (
            <>
              <span className="tabular-nums">{m.width}×{m.height}</span>
              <span className="tabular-nums">{bytes(m.size)}</span>
              {!m.hasAudio && <span className="inline-flex items-center gap-0.5"><VolumeX className="size-3" /> No sound</span>}
            </>
          )}
        </p>
      </div>
      <span className="score shrink-0 text-lg font-semibold tabular-nums">{length(m.duration)}</span>
      <Button size="icon-sm" variant="ghost" disabled={disabled} onClick={onRemove} title="Remove from the list (the file stays)"
        className="shrink-0 text-muted-foreground hover:text-destructive">
        <X /><span className="sr-only">Remove {item.name}</span>
      </Button>
    </li>
  );
}

function PreviewDialog({ item, onClose }: { item: CombineItem; onClose: () => void }) {
  return (
    <div role="dialog" aria-modal aria-label={`Preview of ${item.name}`} className="fixed inset-0 z-50 grid place-items-center bg-board/80 p-4 backdrop-blur-sm"
      onClick={onClose} onKeyDown={(e) => e.key === "Escape" && onClose()}>
      <div className="grid w-full max-w-4xl gap-2" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between gap-2 text-white">
          <p className="truncate font-medium">{item.name}</p>
          <Button size="sm" variant="ghost" className="text-white hover:bg-white/10 hover:text-white" onClick={onClose} autoFocus>
            <X /> Close
          </Button>
        </div>
        <video src={api.itemFileUrl(item.id)} controls autoPlay className="max-h-[75vh] w-full rounded-lg bg-black" />
        <p className="text-sm text-white/60">If it doesn't play here, the format may still combine fine.</p>
      </div>
    </div>
  );
}
