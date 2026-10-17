import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { api, type JobView } from "@/lib/api";

/** Live job state for a session; the server pushes a full JobView on every change. */
export function useJob(videoId: string): JobView | null {
  const qc = useQueryClient();
  useEffect(() => {
    const es = new EventSource(api.eventsUrl(videoId));
    es.onmessage = (e) => qc.setQueryData(["job", videoId], JSON.parse(e.data) as JobView);
    return () => es.close();
  }, [videoId, qc]);
  const { data } = useQuery<JobView | null>({
    queryKey: ["job", videoId],
    queryFn: () => null,
    initialData: null,
    staleTime: Infinity,
  });
  return data;
}
