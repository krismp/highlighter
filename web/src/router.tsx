import { createRootRoute, createRoute, createRouter, Link, Outlet } from "@tanstack/react-router";
import { HomePage } from "@/routes/HomePage";
import { SessionPage } from "@/routes/SessionPage";

const rootRoute = createRootRoute({
  component: () => (
    <div className="min-h-screen bg-background text-foreground">
      <header className="bg-board text-white">
        <div className="mx-auto flex max-w-6xl items-center gap-3 px-4 py-3">
          <Link to="/" className="flex items-center gap-2 rounded-sm focus-visible:ring-2 focus-visible:ring-court focus-visible:outline-none">
            <span className="grid size-7 place-items-center rounded-full bg-court text-sm" aria-hidden>🏀</span>
            <span className="font-heading text-xl font-bold">highlighter</span>
          </Link>
          <span className="hidden text-sm text-white/60 sm:inline">Cut every play out of a match</span>
        </div>
        <div className="h-1 bg-court" aria-hidden />
      </header>
      <main className="mx-auto max-w-6xl px-4 py-8">
        <Outlet />
      </main>
    </div>
  ),
});

const indexRoute = createRoute({ getParentRoute: () => rootRoute, path: "/", component: HomePage });
const sessionRoute = createRoute({ getParentRoute: () => rootRoute, path: "/s/$videoId", component: SessionPage });

export const router = createRouter({ routeTree: rootRoute.addChildren([indexRoute, sessionRoute]) });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
