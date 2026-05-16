import type { ReactNode } from "react";
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
import type { AnyRoute } from "@tanstack/react-router";
import { RootLayout } from "@/routes/__root";
import { HomePage } from "@/routes/home";
import { LoginPage } from "@/routes/login";
import { Placeholder } from "@/routes/placeholder";
import { TransactionsPage, transactionsSearchSchema } from "@/routes/transactions";
import { TransactionDetailPage } from "@/routes/transaction-detail";
import { CategoriesPage } from "@/routes/categories";
import { CategoryNewPage } from "@/routes/category-new";
import { CategoryDetailPage } from "@/routes/category-detail";
import { TagsPage } from "@/routes/tags";
import { TagNewPage } from "@/routes/tag-new";
import { TagDetailPage } from "@/routes/tag-detail";
import {
  ConnectionsPage,
  connectionsSearchSchema,
} from "@/routes/connections";
import {
  ConnectionDetailPage,
  connectionDetailSearchSchema,
} from "@/routes/connection-detail";
import { APIKeysPage, apiKeysSearchSchema } from "@/routes/api-keys";
import { APIKeyNewPage } from "@/routes/api-key-new";
import { APIKeyCreatedPage } from "@/routes/api-key-created";
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

// PAGE_OVERRIDES swaps the default Placeholder for a real page on a given
// path. To ship a page, add one entry — `component`, plus an optional
// `validateSearch` zod schema for typed URL filters/pagination. The route is
// still derived from NAV_LEAVES (single source of truth for paths), but the
// override *replaces* the placeholder rather than adding a second route, so
// there's no way to silently shadow a real page.
interface PageOverride {
  component: () => ReactNode;
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
};

// Detail routes aren't nav leaves, so they're registered explicitly rather
// than derived from NAV_LEAVES. isNavMatch's prefix match keeps the parent
// sidebar item active on each sub-route.
const transactionDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/transactions/$id",
  component: TransactionDetailPage,
});

const categoryNewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/categories/new",
  component: CategoryNewPage,
});

const categoryDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/categories/$id",
  component: CategoryDetailPage,
});

const tagNewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/tags/new",
  component: TagNewPage,
});

const tagDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/tags/$slug",
  component: TagDetailPage,
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
  component: ConnectionDetailPage,
  validateSearch: connectionDetailSearchSchema,
});

// /api-keys/new and /api-keys/created sit beside the list (declared via
// PAGE_OVERRIDES) but aren't nav leaves themselves. The list match still
// keeps the sidebar item active thanks to the prefix match in isNavMatch.
const apiKeyNewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/api-keys/new",
  component: APIKeyNewPage,
});

const apiKeyCreatedRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/api-keys/created",
  component: APIKeyCreatedPage,
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
  transactionDetailRoute,
  categoryNewRoute,
  categoryDetailRoute,
  tagNewRoute,
  tagDetailRoute,
  sandboxRoute,
  connectionDetailRoute,
  apiKeyNewRoute,
  apiKeyCreatedRoute,
  ...pageRoutes,
]);

const router = createRouter({
  routeTree,
  basepath: "/v2",
  defaultPreload: "intent",
});

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 30_000, retry: 1 },
  },
});

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      <TooltipProvider>
        <RouterProvider router={router} />
      </TooltipProvider>
    </QueryClientProvider>
  </React.StrictMode>,
);
