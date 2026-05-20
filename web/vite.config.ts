import { defineConfig, type ProxyOptions } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

const backendPort = process.env.BREADBOX_BACKEND_PORT ?? "8080";
// Vite port derives from the backend port (BACKEND + 1000) when the worktree
// hook injects VITE_PORT, so parallel worktree sessions never collide.
// Defaults to 5173 for solo / non-worktree dev.
const vitePort = Number(process.env.VITE_PORT ?? 5173);

const backendTarget = `http://localhost:${backendPort}`;

// In dev the SPA is served from vitePort but the API lives on backendPort.
// `changeOrigin` rewrites the Host header to the target — but the backend's
// same-origin CSRF check (mw.SameOrigin) also inspects Origin, so it must be
// rewritten to match or every session-authed write 403s in dev. In prod the
// SPA is embedded same-origin, so this proxy never runs.
const apiProxy: ProxyOptions = {
  target: backendTarget,
  changeOrigin: true,
  configure: (proxy) => {
    proxy.on("proxyReq", (proxyReq) => {
      if (proxyReq.getHeader("origin")) {
        proxyReq.setHeader("origin", backendTarget);
      }
    });
  },
};

export default defineConfig({
  base: "/v2/",
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
  server: {
    port: vitePort,
    strictPort: true,
    proxy: {
      "/api": apiProxy,
      "/web/v1": apiProxy,
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    // Vendor chunking. Splits heavy node_modules into stable chunks that
    // cache independently of app code — when app code changes, the user's
    // browser keeps the cached `react`, `tanstack`, `radix` files and only
    // re-downloads the small app delta. Material win for mobile cellular
    // users on repeat visits.
    //
    // Grouping rationale:
    // - `react`: react + react-dom (always loaded; tightest coupling).
    // - `tanstack`: query + router + table — large, change less often than
    //   app code, used across the SPA.
    // - `radix`: every shadcn primitive depends on one of these; bundling
    //   together avoids many small chunks.
    // - Everything else (including `lucide-react`, which icon-picker
    //   dynamically imports per-icon — DON'T group it, Vite auto-splits
    //   each icon into its own ~1 KB chunk that the icon-picker route
    //   pulls in on demand) falls through into the app `index` chunk.
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) return undefined;
          if (id.includes("/react-dom/") || id.match(/[\\/]react[\\/]/)) {
            return "vendor-react";
          }
          if (id.includes("@tanstack/")) return "vendor-tanstack";
          if (id.includes("@radix-ui/") || id.includes("/radix-ui/")) {
            return "vendor-radix";
          }
          return undefined;
        },
      },
    },
  },
});
