import { useCallback, useEffect, useRef, useState } from "react";

declare global {
  interface Window {
    onYouTubeIframeAPIReady?: () => void;
  }
}

let apiReady: Promise<void> | null = null;

function loadIframeApi(): Promise<void> {
  if (typeof YT !== "undefined" && YT.Player) return Promise.resolve();
  apiReady ??= new Promise((resolve) => {
    const previous = window.onYouTubeIframeAPIReady;
    window.onYouTubeIframeAPIReady = () => {
      previous?.();
      resolve();
    };
    const script = document.createElement("script");
    script.src = "https://www.youtube.com/iframe_api";
    document.head.appendChild(script);
  });
  return apiReady;
}

interface Range {
  id: string;
  start: number;
  end: number;
}

/**
 * Embeds the YouTube player. preview() plays [start, start+duration] and auto-pauses;
 * previewing the same clip again resumes from the playhead while it is still inside
 * the clip, and extend() moves the end of the running preview without seeking.
 * Pass enabled=false until the element bound to containerRef is rendered.
 */
export function useYouTubePlayer(videoId: string, enabled: boolean) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const playerRef = useRef<YT.Player | null>(null);
  const rangeRef = useRef<Range | null>(null); // last previewed clip, kept after it stops
  const activeRef = useRef(false);
  const [ready, setReady] = useState(false);
  const [previewing, setPreviewing] = useState<string | null>(null);
  const [playhead, setPlayhead] = useState(0);

  const stop = useCallback((pause: boolean) => {
    activeRef.current = false;
    if (pause) playerRef.current?.pauseVideo();
    setPreviewing(null);
  }, []);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    let timer: number | undefined;
    loadIframeApi().then(() => {
      if (cancelled || !containerRef.current) return;
      const mount = document.createElement("div");
      containerRef.current.appendChild(mount);
      const player = new YT.Player(mount, {
        videoId,
        width: "100%",
        height: "100%",
        playerVars: { rel: 0, playsinline: 1 },
        events: { onReady: () => !cancelled && setReady(true) },
      });
      playerRef.current = player;
      timer = window.setInterval(() => {
        if (typeof player.getCurrentTime !== "function") return;
        const t = player.getCurrentTime();
        setPlayhead((prev) => (Math.abs(prev - t) >= 0.05 ? t : prev));
        const r = rangeRef.current;
        if (!activeRef.current || !r) return;
        // Ignore a stale position from before a seek registers (t < start - 0.5).
        if ((t >= r.end && t >= r.start - 0.5) || player.getPlayerState() === YT.PlayerState.ENDED) {
          player.pauseVideo();
          activeRef.current = false;
          setPreviewing(null);
        }
      }, 100);
    });
    return () => {
      cancelled = true;
      window.clearInterval(timer);
      activeRef.current = false;
      rangeRef.current = null;
      playerRef.current?.destroy();
      playerRef.current = null;
      if (containerRef.current) containerRef.current.innerHTML = "";
      setReady(false);
      setPreviewing(null);
    };
  }, [videoId, enabled]);

  const preview = useCallback((id: string, start: number, duration: number) => {
    const player = playerRef.current;
    if (!player) return;
    const end = start + duration;
    const t = player.getCurrentTime();
    const resume = rangeRef.current?.id === id && t >= start - 0.25 && t < end - 0.1;
    if (!resume) player.seekTo(start, true);
    rangeRef.current = { id, start, end };
    activeRef.current = true;
    player.playVideo();
    setPreviewing(id);
  }, []);

  /** Updates the bounds of the clip being previewed, keeping playback where it is. */
  const extend = useCallback((id: string, start: number, duration: number) => {
    const r = rangeRef.current;
    if (!r || r.id !== id) return;
    rangeRef.current = { id, start, end: start + duration };
  }, []);

  const stopPreview = useCallback(() => stop(true), [stop]);
  const currentTime = useCallback(() => playerRef.current?.getCurrentTime() ?? 0, []);
  const range = previewing ? rangeRef.current : null;

  return { containerRef, ready, previewing, playhead, range, preview, extend, stopPreview, currentTime };
}
