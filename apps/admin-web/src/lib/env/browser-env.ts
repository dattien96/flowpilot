type EnvSource = Record<string, string | undefined>;

function currentEnv(): EnvSource {
  return (import.meta as ImportMeta & { env?: EnvSource }).env ?? {};
}

function readRequiredEnv(
  name: "SUPABASE_API_URL" | "SUPABASE_API_KEY" | "SUPABASE_API_EDGE_FUNCTION_URL",
) {
  const value = currentEnv()[name];

  if (!value) {
    throw new Error(`Missing required environment variable: ${name}`);
  }

  return value;
}

export function hasSupabaseEnv() {
  const env = currentEnv();
  return Boolean(env.SUPABASE_API_URL && env.SUPABASE_API_KEY);
}

export function getSupabaseUrl() {
  return readRequiredEnv("SUPABASE_API_URL");
}

export function getSupabaseAnonKey() {
  return readRequiredEnv("SUPABASE_API_KEY");
}

export function getSupabaseEdgeFunctionUrl() {
  return readRequiredEnv("SUPABASE_API_EDGE_FUNCTION_URL");
}

export function getLocalRunnerBaseUrl() {
  return currentEnv().VITE_LOCAL_RUNNER_URL ?? "http://localhost:9100";
}
