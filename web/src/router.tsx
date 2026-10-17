import { createRootRoute, createRoute, createRouter, Link, Outlet } from "@tanstack/react-router";
import { HomePage } from "@/routes/HomePage";
import { SessionPage } from "@/routes/SessionPage";

const rootRoute = createRootRoute({
  component: () => (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b">
        <div className="mx-auto max-w-6xl px-4 py-3">
          <Link to="/" className="font-semibold tracking-tight">🏀 highlighter</Link>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-4 py-6">
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
