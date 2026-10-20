import { createRootRoute, createRoute, createRouter, Link, Outlet } from "@tanstack/react-router";
import { DropOverlay } from "@/components/DropOverlay";
import { OfflineBanner } from "@/components/OfflineBanner";
import { QuitButton } from "@/components/QuitButton";
import { UpdateBanner } from "@/components/Updates";
import { CombinePage } from "@/routes/CombinePage";
import { HomePage } from "@/routes/HomePage";
import { SessionPage } from "@/routes/SessionPage";
import { SettingsPage } from "@/routes/SettingsPage";
import { Film, Settings } from "lucide-react";

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
          <Link to="/combine" className="ml-auto flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-sm text-white/70 hover:bg-white/10 hover:text-white [&.active]:bg-white/10 [&.active]:text-white" title="Join videos into one">
            <Film className="size-3.5" /> Combine
          </Link>
          <Link to="/settings" className="flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-sm text-white/70 hover:bg-white/10 hover:text-white [&.active]:text-white" title="Settings">
            <Settings className="size-3.5" /> Settings
          </Link>
          <QuitButton />
        </div>
        <div className="h-1 bg-court" aria-hidden />
      </header>
      <UpdateBanner />
      <OfflineBanner />
      <main className="mx-auto max-w-6xl px-4 py-8">
        <Outlet />
      </main>
      <DropOverlay />
    </div>
  ),
});

const indexRoute = createRoute({ getParentRoute: () => rootRoute, path: "/", component: HomePage });
const settingsRoute = createRoute({ getParentRoute: () => rootRoute, path: "/settings", component: SettingsPage });
const combineRoute = createRoute({ getParentRoute: () => rootRoute, path: "/combine", component: CombinePage });
const sessionRoute = createRoute({ getParentRoute: () => rootRoute, path: "/s/$videoId", component: SessionPage });

export const router = createRouter({ routeTree: rootRoute.addChildren([indexRoute, settingsRoute, combineRoute, sessionRoute]) });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}
