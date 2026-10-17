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

/**
 * Embeds the YouTube player; preview() plays [start, start+duration] and auto-pauses.
 * Pass enabled=false until the element bound to containerRef is rendered.
 */
export function useYouTubePlayer(videoId: string, enabled: boolean) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const playerRef = useRef<YT.Player | null>(null);
  const timerRef = useRef<number | null>(null);
  const [ready, setReady] = useState(false);
  const [previewing, setPreviewing] = useState<string | null>(null);

  const clearTimer = () => {
    if (timerRef.current !== null) window.clearInterval(timerRef.current);
    timerRef.current = null;
  };

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    loadIframeApi().then(() => {
      if (cancelled || !containerRef.current) return;
      const mount = document.createElement("div");
      containerRef.current.appendChild(mount);
      playerRef.current = new YT.Player(mount, {
        videoId,
        width: "100%",
        height: "100%",
        playerVars: { rel: 0, playsinline: 1 },
        events: { onReady: () => !cancelled && setReady(true) },
      });
    });
    return () => {
      cancelled = true;
      clearTimer();
      playerRef.current?.destroy();
      playerRef.current = null;
      if (containerRef.current) containerRef.current.innerHTML = "";
      setReady(false);
    };
  }, [videoId, enabled]);

  const stopPreview = useCallback(() => {
    clearTimer();
    playerRef.current?.pauseVideo();
    setPreviewing(null);
  }, []);

  const preview = useCallback((id: string, start: number, duration: number) => {
    const player = playerRef.current;
    if (!player) return;
    clearTimer();
    player.seekTo(start, true);
    player.playVideo();
    setPreviewing(id);
    const end = start + duration;
    timerRef.current = window.setInterval(() => {
      if (player.getCurrentTime() >= end) {
        player.pauseVideo();
        clearTimer();
        setPreviewing(null);
      }
    }, 100);
  }, []);

  const currentTime = useCallback(() => playerRef.current?.getCurrentTime() ?? 0, []);

  return { containerRef, ready, previewing, preview, stopPreview, currentTime };
}
