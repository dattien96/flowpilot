interface ImportMetaEnv {
  readonly VITE_LOCAL_RUNNER_URL?: string;
  readonly SUPABASE_API_EDGE_FUNCTION_URL?: string;
  readonly SUPABASE_API_KEY?: string;
  readonly SUPABASE_API_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
