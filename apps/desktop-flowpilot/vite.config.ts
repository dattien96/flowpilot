import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import electron from "vite-plugin-electron/simple";
import path from "node:path";

// Phase 1 mock MVP (04-01 Part A): standard Vite renderer (matching admin-web)
// plus Electron main/preload built by vite-plugin-electron. Zero backend.
export default defineConfig({
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
      "@flowpilot/client-core": path.resolve(
        __dirname,
        "../../packages/flowpilot-client-core/src/index.ts",
      ),
    },
  },
  plugins: [
    react(),
    electron({
      main: { entry: "electron/main.ts" },
      preload: { input: path.join(__dirname, "electron/preload.ts") },
      // Renderer can use Node built-ins if ever needed; harmless for the mock.
      renderer: {},
    }),
  ],
});
