import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

const backendPort = process.env.BREADBOX_BACKEND_PORT ?? "8080";

export default defineConfig({
  base: "/v2/",
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: { "@": path.resolve(__dirname, "./src") },
  },
  server: {
    port: 5173,
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
