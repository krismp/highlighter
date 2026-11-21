import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";

/** The recent videos, refreshed every couple of seconds while any is still processing. */
export function useRecent() {
  return useQuery({
    queryKey: ["recent"],
    queryFn: api.recent,
    refetchInterval: (q) => (q.state.data?.some((r) => r.status === "processing") ? 2000 : false),
    meta: { errorTitle: "Couldn't load recent videos" },
  });
}
