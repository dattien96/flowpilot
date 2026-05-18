import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";

export default defineConfig({
  envDir: path.resolve(__dirname, "../.."),
  envPrefix: [
    "VITE_",
    "SUPABASE_API_URL",
    "SUPABASE_API_KEY",
    "SUPABASE_API_EDGE_FUNCTION_URL",
  ],
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    host: "0.0.0.0",
    port: 3001,
    strictPort: true,
    watch: {
      usePolling: true,
      interval: 300,
    },
    hmr: {
      host: "localhost",
      clientPort: 3001,
    },
  },
});
