import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";

function useOS(): string | undefined {
  const health = useQuery({ queryKey: ["health"], queryFn: api.health, staleTime: Infinity, meta: { errorTitle: "Couldn't check the bundled tools" } });
  return health.data?.os;
}

/** What the OS calls its trash: "Recycle Bin" on Windows. */
export function useTrashName(): string {
  return useOS() === "windows" ? "Recycle Bin" : "Trash";
}

/** What the OS calls its file browser, for "Show in …" buttons. */
export function useFileManager(): string {
  switch (useOS()) {
    case "darwin":
      return "Finder";
    case "windows":
      return "File Explorer";
    default:
      return "folder";
  }
}
