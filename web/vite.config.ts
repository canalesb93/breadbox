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
      "/health": apiProxy,
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
