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
import { z } from "zod";
import "@/globals.css";

const rootRoute = createRootRoute({ component: RootLayout });

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

const placeholderRoutes = NAV_LEAVES.filter((leaf) => leaf.to !== "/").map((leaf) =>
  createRoute({
    getParentRoute: () => rootRoute,
    path: leaf.to,
    component: () => <Placeholder title={leaf.title} />,
  }),
);

const routeTree = rootRoute.addChildren([indexRoute, loginRoute, ...placeholderRoutes]);

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
