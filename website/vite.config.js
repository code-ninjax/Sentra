import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  base: "./",
  server: {
    // listen on all interfaces so tunnel tools (OutRay etc.) can reach it
    host: true,
    // Vite blocks unknown Host headers; allow tunnel subdomains
    allowedHosts: [".outray.app"],
  },
  build: {
    outDir: "dist",
    assetsInlineLimit: 0,
  },
});
