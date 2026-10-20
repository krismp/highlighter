import { useQuery } from "@tanstack/react-query";
import { inDesktopApp } from "@/lib/desktop";
import { checkForUpdate } from "@/lib/updates";

/** The launch check, once per session; failures (offline, say) stay quiet here and show in Settings. */
export function useUpdateCheck() {
  return useQuery({ queryKey: ["update"], queryFn: checkForUpdate, enabled: inDesktopApp, staleTime: Infinity, retry: false, meta: { inline: true } });
}
