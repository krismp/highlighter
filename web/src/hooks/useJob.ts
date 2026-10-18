import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { api, type JobView } from "@/lib/api";
import { reportError } from "@/lib/errors";

/** Live job state for a session; the server pushes a full JobView on every change. */
export function useJob(videoId: string): JobView | null {
  const qc = useQueryClient();
  useEffect(() => {
    const es = new EventSource(api.eventsUrl(videoId));
    let lost: number | undefined;
    es.onopen = () => window.clearTimeout(lost);
    es.onmessage = (e) => {
      try {
        qc.setQueryData(["job", videoId], JSON.parse(e.data) as JobView);
      } catch (err) {
        reportError("Couldn't read processing progress", err, `event data: ${String(e.data).slice(0, 300)}`);
      }
    };
    // EventSource reconnects by itself; only report a connection that stays down.
    es.onerror = () => {
      window.clearTimeout(lost);
      lost = window.setTimeout(() => {
        if (es.readyState !== EventSource.OPEN) {
          reportError("Lost live progress updates",
            "The connection to highlighter's local server dropped, so progress on this page may be out of date. Reload the page to catch up.",
            `GET ${api.eventsUrl(videoId)} (readyState ${es.readyState})`);
        }
      }, 5000);
    };
    return () => {
      window.clearTimeout(lost);
      es.close();
    };
  }, [videoId, qc]);
  const { data } = useQuery<JobView | null>({
    queryKey: ["job", videoId],
    queryFn: () => null,
    initialData: null,
    staleTime: Infinity,
  });
  return data;
}
