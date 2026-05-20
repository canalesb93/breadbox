import React from "react";
import ReactDOM from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  RouterProvider,
  createRouter,
  createRootRoute,
  createRoute,
  lazyRouteComponent,
} from "@tanstack/react-router";
import { TooltipProvider } from "@/components/ui/tooltip";
import { ThemeProvider } from "@/components/theme-provider";
import type { AnyRoute } from "@tanstack/react-router";
import { RootLayout } from "@/routes/__root";
import { HomePage } from "@/routes/home";
import { LoginPage } from "@/routes/login";
import { SetupAccountPage } from "@/routes/setup-account";
import { Placeholder } from "@/routes/placeholder";
import { NotFoundPage } from "@/routes/not-found";
import { ErrorPage } from "@/routes/error";
// List pages are eager (most-visited on initial load); detail / edit /
// new / form pages lazy-load via `lazyRouteComponent` so they stay out
// of the boot chunk. Search-param schemas stay imported because the
// router needs them to validate URLs even before the lazy component
// resolves. Pattern matches the existing sandbox route.
import { TransactionsPage, transactionsSearchSchema } from "@/routes/transactions";
import { CategoriesPage } from "@/routes/categories";
import { TagsPage } from "@/routes/tags";
import {
  ConnectionsPage,
  connectionsSearchSchema,
} from "@/routes/connections";
import { connectionDetailSearchSchema } from "@/routes/connection-detail";
import { APIKeysPage, apiKeysSearchSchema } from "@/routes/api-keys";
import { ProvidersPage } from "@/routes/providers";
import { AccountsPage, accountsSearchSchema } from "@/routes/accounts";
import { accountDetailSearchSchema } from "@/routes/account-detail";
import { RulesPage, rulesSearchSchema } from "@/routes/rules";
import { AgentsPage } from "@/routes/agents";
import { agentsNewSearchSchema } from "@/routes/agents.new";
import { agentsRunsSearchSchema } from "@/routes/agents.runs";
import { agentRunsSearchSchema } from "@/routes/agents.$slug.runs";
import { promptsBuildSearchSchema } from "@/routes/prompts.build";
import { NAV_LEAVES } from "@/lib/nav";
import { baseSearchSchema } from "@/lib/modals";
import { z } from "zod";
import "@/globals.css";

// baseSearchSchema on the root route makes the modal params (`m`/`ms`) valid
// search params everywhere; page-level schemas merge with it.
const rootRoute = createRootRoute({
  component: RootLayout,
  validateSearch: baseSearchSchema,
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: HomePage,
});

const loginSearchSchema = z.object({ redirect: z.string().optional() });

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  component: LoginPage,
  validateSearch: loginSearchSchema,
});

const setupAccountRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/setup-account/$token",
  component: SetupAccountPage,
});

// PAGE_OVERRIDES swaps the default Placeholder for a real page on a given
// path. To ship a page, add one entry — `component`, plus an optional
// `validateSearch` zod schema for typed URL filters/pagination. The route is
// still derived from NAV_LEAVES (single source of truth for paths), but the
// override *replaces* the placeholder rather than adding a second route, so
// there's no way to silently shadow a real page.
interface PageOverride {
  // Accepts both eager components and `lazyRouteComponent` (async) shapes —
  // mirrors what `createRoute({ component })` itself accepts.
  component: Parameters<typeof createRoute>[0]["component"];
  validateSearch?: Parameters<typeof createRoute>[0]["validateSearch"];
}

const PAGE_OVERRIDES: Record<string, PageOverride> = {
  "/transactions": {
    component: TransactionsPage,
    validateSearch: transactionsSearchSchema,
  },
  "/categories": {
    component: CategoriesPage,
  },
  "/tags": {
    component: TagsPage,
  },
  "/connections": {
    component: ConnectionsPage,
    validateSearch: connectionsSearchSchema,
  },
  "/api-keys": {
    component: APIKeysPage,
    validateSearch: apiKeysSearchSchema,
  },
  "/providers": {
    component: ProvidersPage,
  },
  "/accounts": {
    component: AccountsPage,
    validateSearch: accountsSearchSchema,
  },
  "/rules": {
    component: RulesPage,
    validateSearch: rulesSearchSchema,
  },
  "/agents": {
    component: AgentsPage,
  },
  "/prompts/build": {
    component: lazyRouteComponent(
      () => import("@/routes/prompts.build"),
      "PromptsBuildPage",
    ),
    validateSearch: promptsBuildSearchSchema,
  },
};

// Detail routes aren't nav leaves, so they're registered explicitly rather
// than derived from NAV_LEAVES. isNavMatch's prefix match keeps the parent
// sidebar item active on each sub-route.
const transactionDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/transactions/$id",
  component: lazyRouteComponent(
    () => import("@/routes/transaction-detail"),
    "TransactionDetailPage",
  ),
});

const categoryNewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/categories/new",
  component: lazyRouteComponent(
    () => import("@/routes/category-new"),
    "CategoryNewPage",
  ),
});

const categoryDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/categories/$id",
  component: lazyRouteComponent(
    () => import("@/routes/category-detail"),
    "CategoryDetailPage",
  ),
});

const tagNewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/tags/new",
  component: lazyRouteComponent(() => import("@/routes/tag-new"), "TagNewPage"),
});

const tagDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/tags/$slug",
  component: lazyRouteComponent(
    () => import("@/routes/tag-detail"),
    "TagDetailPage",
  ),
});

// The design-system sandbox — a dev/reference gallery, not a nav leaf.
// Lazy-loaded so its fixtures + section code stay out of the main bundle.
const sandboxRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/sandbox",
  component: lazyRouteComponent(() => import("@/routes/sandbox"), "SandboxPage"),
});

const connectionDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/connections/$id",
  component: lazyRouteComponent(
    () => import("@/routes/connection-detail"),
    "ConnectionDetailPage",
  ),
  validateSearch: connectionDetailSearchSchema,
});

// /api-keys/new and /api-keys/created sit beside the list (declared via
// PAGE_OVERRIDES) but aren't nav leaves themselves. The list match still
// keeps the sidebar item active thanks to the prefix match in isNavMatch.
const apiKeyNewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/api-keys/new",
  component: lazyRouteComponent(
    () => import("@/routes/api-key-new"),
    "APIKeyNewPage",
  ),
});

const apiKeyCreatedRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/api-keys/created",
  component: lazyRouteComponent(
    () => import("@/routes/api-key-created"),
    "APIKeyCreatedPage",
  ),
});

const accountDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/accounts/$id",
  component: lazyRouteComponent(
    () => import("@/routes/account-detail"),
    "AccountDetailPage",
  ),
  validateSearch: accountDetailSearchSchema,
});

// /rules/new and /rules/$id/edit share one form component (RuleFormPage) —
// the `mode` distinguishes them. Mode-specific wrappers exported from
// rule-form.tsx (`RuleNewPage` / `RuleEditPage`) so lazyRouteComponent
// can import a single module per entry point. Declared before /rules/$id
// so the more specific path wins the route match.
const ruleNewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/rules/new",
  component: lazyRouteComponent(
    () => import("@/routes/rule-form"),
    "RuleNewPage",
  ),
});

const ruleEditRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/rules/$id/edit",
  component: lazyRouteComponent(
    () => import("@/routes/rule-form"),
    "RuleEditPage",
  ),
});

const ruleDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/rules/$id",
  component: lazyRouteComponent(
    () => import("@/routes/rule-detail"),
    "RuleDetailPage",
  ),
});

// /agents/new, /agents/runs, /agents/$slug/edit and /agents/$slug/runs —
// declared before the more general /agents nav-leaf route so chi-style
// longest-match prevails. /agents/runs is a static path and must precede
// the dynamic /agents/$slug/* family for the same reason.
const agentNewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/agents/new",
  component: lazyRouteComponent(
    () => import("@/routes/agents.new"),
    "AgentNewPage",
  ),
  validateSearch: agentsNewSearchSchema,
});

const agentsRunsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/agents/runs",
  component: lazyRouteComponent(
    () => import("@/routes/agents.runs"),
    "AgentsRunsPage",
  ),
  validateSearch: agentsRunsSearchSchema,
});

const agentEditRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/agents/$slug/edit",
  component: lazyRouteComponent(
    () => import("@/routes/agents.$slug.edit"),
    "AgentEditPage",
  ),
});

const agentRunsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/agents/$slug/runs",
  component: lazyRouteComponent(
    () => import("@/routes/agents.$slug.runs"),
    "AgentRunsPage",
  ),
  validateSearch: agentRunsSearchSchema,
});

const pageRoutes = NAV_LEAVES.flatMap(({ leaf }) => {
  if (leaf.kind !== "link" || leaf.to === "/") return [];
  const override = PAGE_OVERRIDES[leaf.to];
  const component =
    override?.component ?? (() => <Placeholder title={leaf.title} />);
  return [
    createRoute({
      getParentRoute: () => rootRoute,
      path: leaf.to,
      component,
      validateSearch: override?.validateSearch,
    }),
  ];
}) as AnyRoute[];

const routeTree = rootRoute.addChildren([
  indexRoute,
  loginRoute,
  setupAccountRoute,
  transactionDetailRoute,
  categoryNewRoute,
  categoryDetailRoute,
  tagNewRoute,
  tagDetailRoute,
  sandboxRoute,
  connectionDetailRoute,
  apiKeyNewRoute,
  apiKeyCreatedRoute,
  accountDetailRoute,
  ruleNewRoute,
  ruleEditRoute,
  ruleDetailRoute,
  agentNewRoute,
  agentsRunsRoute,
  agentEditRoute,
  agentRunsRoute,
  ...pageRoutes,
]);

const router = createRouter({
  routeTree,
  basepath: "/v2",
  defaultPreload: "intent",
  // Default not-found + error components render in place of <Outlet/> inside
  // the authenticated shell, so the sidebar/topbar/command palette stay live
  // and the user has a way out without a hard reload. See the route files
  // for the visual contract.
  defaultNotFoundComponent: NotFoundPage,
  defaultErrorComponent: ({ error, reset }) => (
    <ErrorPage error={error} reset={reset} />
  ),
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
      // Explicit gcTime — TanStack's default is 5min, but stating it
      // here locks the contract: inactive (no observer) query data is
      // dropped 5 minutes after the last component using it unmounts.
      // Caps long-iOS-session memory growth; individual queries can
      // override (e.g. `["me"]` uses `Infinity` because auth state is a
      // single small object the auth gate reads on every render).
      gcTime: 5 * 60 * 1000,
    },
  },
});

// iOS Safari restores the SPA from bfcache on swipe-back: the DOM snapshot is
// reconstituted and JS resumes mid-state. Without intervention the stale React
// + React-Query state can trigger a 401-refetch → /login redirect race during
// Safari's restore animation, presenting as a frozen / blank page. The fix
// has two parts:
//
//  1. Re-validate the session (`["me"]`) so a 401 fires from a cold loader
//     state instead of mid-restore animation; the auth gate in `__root.tsx`
//     waits for `document.visibilityState === "visible"` before redirecting.
//
//  2. Invalidate the router so route loaders + search-param validation re-run
//     against the now-restored DOM.
//
// We deliberately do NOT call `queryClient.invalidateQueries()` (no args) —
// that fires a refetch storm against every cached query on the visible page
// (transactions list alone has 10+), producing a measurable 1-2s freeze
// after the restore. TanStack Query's 30s staleTime already handles freshness
// for normal data; the bfcache restore window is usually shorter than that.
// Routes that need stricter freshness can opt into their own `pageshow`
// handler or use a shorter staleTime.
//
// Don't `queryClient.clear()` — that drops cached data instantly. Don't add a
// `beforeunload`/`unload` handler — that would disable bfcache entirely.
if (typeof window !== "undefined") {
  window.addEventListener("pageshow", (event) => {
    if (event.persisted) {
      router.invalidate();
      queryClient.invalidateQueries({ queryKey: ["me"] });
    }
  });
}

// Web Vitals listener — logs LCP / INP-proxy / CLS to the console when
// `VITE_REPORT_VITALS` is enabled (defaults to on in dev, off in prod).
// See `web/src/lib/web-vitals.ts`. Future iteration: pipe to a backend
// `/api/v1/web-vitals` endpoint for real perf baseline tracking.
void import("@/lib/web-vitals").then((m) => m.startWebVitals());

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider delayDuration={200}>
          <RouterProvider router={router} />
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  </React.StrictMode>,
);
