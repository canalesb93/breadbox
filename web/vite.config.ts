import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

const backendPort = process.env.BREADBOX_BACKEND_PORT ?? "8081";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: `http://localhost:${backendPort}`,
        changeOrigin: true,
      },
    },
  },
});
