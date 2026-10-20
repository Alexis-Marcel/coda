import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const BACKEND = "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": { target: BACKEND },
      "/ws": { target: BACKEND, ws: true },
      "/healthz": { target: BACKEND },
    },
  },
});
