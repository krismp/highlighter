import { useNavigate } from "@tanstack/react-router";
import { Film } from "lucide-react";
import { useEffect, useState } from "react";
import { useAddVideos } from "@/hooks/useCombine";
import { isVideoPath, onFileDrag } from "@/lib/desktop";

/**
 * Videos dropped anywhere on the window go to Combine: while files are
 * dragged over the window it says what will happen, and a drop adds them and
 * opens the Combine page.
 */
export function DropOverlay() {
  const navigate = useNavigate();
  const add = useAddVideos();
  const [dragging, setDragging] = useState<number | null>(null); // videos being dragged, null when nothing is

  useEffect(() => onFileDrag((e) => {
    if (e.type === "enter") setDragging(e.paths.filter(isVideoPath).length);
    else if (e.type === "leave") setDragging(null);
    else if (e.type === "drop") {
      setDragging(null);
      if (e.paths.length === 0) return;
      add.mutate(e.paths);
      navigate({ to: "/combine" });
    }
  }), [add.mutate, navigate]); // eslint-disable-line react-hooks/exhaustive-deps

  if (dragging === null) return null;
  return (
    <div className="pointer-events-none fixed inset-0 z-50 grid place-items-center bg-board/75 p-6 backdrop-blur-[2px]" aria-live="polite">
      <div className="grid justify-items-center gap-3 rounded-2xl border-2 border-dashed border-court bg-board/90 px-12 py-10 text-center text-white shadow-2xl">
        <Film className="size-10 text-court" />
        <p className="font-heading text-3xl font-bold">
          {dragging === 0 ? "Drop to add these files" : `Drop to add ${dragging} video${dragging === 1 ? "" : "s"}`}
        </p>
        <p className="text-white/70">They go to the end of your Combine list.</p>
      </div>
    </div>
  );
}
