import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

const backend = "http://localhost:8080";

export default defineConfig({
  plugins: [react()],
  build: { outDir: "../server/web/dist", emptyOutDir: true },
  server: {
    proxy: {
      "/api": backend,
      "/stream.mjpg": backend,
      "/media": backend,
    },
  },
});
