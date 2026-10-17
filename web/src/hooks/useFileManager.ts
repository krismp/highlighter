import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";

/** What the OS calls its file browser, for "Show in …" buttons. */
export function useFileManager(): string {
  const health = useQuery({ queryKey: ["health"], queryFn: api.health, staleTime: Infinity });
  switch (health.data?.os) {
    case "darwin":
      return "Finder";
    case "windows":
      return "File Explorer";
    default:
      return "folder";
  }
}
