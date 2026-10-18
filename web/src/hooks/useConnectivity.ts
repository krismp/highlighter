import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { api, type Connectivity } from "@/lib/api";

/**
 * Whether the internet (YouTube) is reachable, as checked by the local server.
 * Rechecks every 30s, every 5s while offline, and right away when the OS
 * reports the network going up or down. undefined until the first check.
 */
export function useConnectivity() {
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: ["connectivity"],
    queryFn: api.connectivity,
    refetchInterval: (q) => (q.state.data?.online === false ? 5_000 : 30_000),
    meta: { errorTitle: "Couldn't check the internet connection" },
  });
  useEffect(() => {
    const offline = () => qc.setQueryData<Connectivity>(["connectivity"], { online: false, error: "This computer is offline." });
    const online = () => qc.invalidateQueries({ queryKey: ["connectivity"] });
    window.addEventListener("offline", offline);
    window.addEventListener("online", online);
    return () => {
      window.removeEventListener("offline", offline);
      window.removeEventListener("online", online);
    };
  }, [qc]);
  return { online: query.data?.online, error: query.data?.error ?? "", checking: query.isFetching, recheck: () => query.refetch() };
}
