interface ImportMetaEnv {
  readonly VITE_ADMIN_WEB_URL?: string;
  readonly VITE_LOCAL_RUNNER_URL?: string;
  readonly FLOWPILOT_RUNNER_PORT?: string;
  readonly FLOWPILOT_RUNNER_URL?: string;
  readonly SUPABASE_API_EDGE_FUNCTION_URL?: string;
  readonly SUPABASE_API_KEY?: string;
  readonly SUPABASE_API_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
