/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Dev proxy: the master serves /api (REST) and /live (WebSocket) with no CORS,
// so during `vite dev` we proxy both to a locally-running master.
// In production nginx fronts the same paths (web/nginx.conf).
const MASTER = process.env.MASTER_ORIGIN || "http://127.0.0.1:8080";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      "/api": { target: MASTER, changeOrigin: true },
      "/live": { target: MASTER.replace(/^http/, "ws"), ws: true, changeOrigin: true },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test-setup.ts"],
  },
});
