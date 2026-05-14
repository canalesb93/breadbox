import type { ReactNode } from "react";
import React from "react";
import ReactDOM from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  RouterProvider,
  createRouter,
  createRootRoute,
  createRoute,
} from "@tanstack/react-router";
import { TooltipProvider } from "@/components/ui/tooltip";
import { RootLayout } from "@/routes/__root";
import { HomePage } from "@/routes/home";
import { LoginPage } from "@/routes/login";
import { Placeholder } from "@/routes/placeholder";
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

// PAGE_COMPONENTS overrides the default Placeholder for a given path. To ship
// a real page, add one entry here — e.g. `"/transactions": TransactionsPage`.
// The page route is still derived from NAV_LEAVES (single source of truth for
// paths), but the override *replaces* the placeholder rather than adding a
// second route alongside it, so there's no way to silently shadow a real page.
const PAGE_COMPONENTS: Record<string, () => ReactNode> = {
  // "/transactions": TransactionsPage,
};

const pageRoutes = NAV_LEAVES.flatMap(({ leaf }) => {
  if (leaf.kind !== "link" || leaf.to === "/") return [];
  const override = PAGE_COMPONENTS[leaf.to];
  const component = override ?? (() => <Placeholder title={leaf.title} />);
  return [
    createRoute({
      getParentRoute: () => rootRoute,
      path: leaf.to,
      component,
    }),
  ];
});

const routeTree = rootRoute.addChildren([
  indexRoute,
  loginRoute,
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
