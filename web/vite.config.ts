import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

const backendPort = process.env.BREADBOX_BACKEND_PORT ?? "8080";
// Vite port derives from the backend port (BACKEND + 1000) when the worktree
// hook injects VITE_PORT, so parallel worktree sessions never collide.
// Defaults to 5173 for solo / non-worktree dev.
const vitePort = Number(process.env.VITE_PORT ?? 5173);

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
      "/api": { target: `http://localhost:${backendPort}`, changeOrigin: true },
      "/web/v1": { target: `http://localhost:${backendPort}`, changeOrigin: true },
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
  },
});
