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
import { Placeholder } from "@/routes/placeholder";
import { NAV } from "@/lib/nav";
import "@/globals.css";

const rootRoute = createRootRoute({ component: RootLayout });

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: HomePage,
});

const placeholderRoutes = NAV.flatMap((group) =>
  group.items
    .filter((item) => item.to !== "/")
    .map((item) =>
      createRoute({
        getParentRoute: () => rootRoute,
        path: item.to,
        component: () => <Placeholder title={item.title} />,
      }),
    ),
);

const routeTree = rootRoute.addChildren([indexRoute, ...placeholderRoutes]);

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
