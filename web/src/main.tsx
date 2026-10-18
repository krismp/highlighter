import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { ErrorAlerts } from "@/components/ErrorAlerts";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { api } from "@/lib/api";
import { reportContext, reportError } from "@/lib/errors";
import { router } from "@/router";
import "./index.css";

declare module "@tanstack/react-query" {
  interface Register {
    // errorTitle names the failed action in the alert; inline: the component shows the error itself.
    queryMeta: { errorTitle?: string; inline?: boolean };
    mutationMeta: { errorTitle?: string; inline?: boolean };
  }
}

// No silent failures: every failed request becomes a closable, copyable alert.
const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  queryCache: new QueryCache({
    onError: (err, query) => {
      if (!query.meta?.inline) reportError(query.meta?.errorTitle ?? "Couldn't load data", err);
    },
  }),
  mutationCache: new MutationCache({
    onError: (err, _vars, _ctx, mutation) => {
      if (!mutation.meta?.inline) reportError(mutation.meta?.errorTitle ?? "Something went wrong", err);
    },
  }),
});

window.addEventListener("error", (e) => reportError("Unexpected error", e.error ?? e.message));
window.addEventListener("unhandledrejection", (e) => reportError("Unexpected error", e.reason));

// Version and OS for error reports. A failure is already reported by the query cache.
queryClient.fetchQuery({ queryKey: ["health"], queryFn: api.health, meta: { errorTitle: "Couldn't check the bundled tools" } })
  .then((h) => Object.assign(reportContext, { version: h.version, os: h.os }), () => {});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ErrorBoundary>
        <RouterProvider router={router} />
      </ErrorBoundary>
      <ErrorAlerts />
    </QueryClientProvider>
  </StrictMode>,
);
