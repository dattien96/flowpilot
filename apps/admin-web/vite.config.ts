import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import path from "node:path";
import { fileURLToPath } from "node:url";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");

  return {
    plugins: [react()],
    define: {
      "import.meta.env.SUPABASE_API_URL": JSON.stringify(env.SUPABASE_API_URL),
      "import.meta.env.SUPABASE_API_KEY": JSON.stringify(env.SUPABASE_API_KEY),
      "import.meta.env.FLOWPILOT_RUNNER_URL": JSON.stringify(env.FLOWPILOT_RUNNER_URL),
    },
    resolve: {
      alias: {
        "@": path.resolve(fileURLToPath(new URL(".", import.meta.url)), "src"),
      },
    },
  };
});
