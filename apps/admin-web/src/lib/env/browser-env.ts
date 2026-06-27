type EnvSource = Record<string, string | undefined>;
type EnvGlobal = typeof globalThis & {
  __FLOWPILOT_VITE_ENV__?: EnvSource;
};

function currentEnv(): EnvSource {
  const candidate = globalThis as EnvGlobal;
  if (candidate.__FLOWPILOT_VITE_ENV__) {
    return candidate.__FLOWPILOT_VITE_ENV__;
  }
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
  const env = currentEnv();
  if (env.VITE_LOCAL_RUNNER_URL) return env.VITE_LOCAL_RUNNER_URL;
  if (env.FLOWPILOT_RUNNER_URL) return env.FLOWPILOT_RUNNER_URL;
  if (env.FLOWPILOT_RUNNER_PORT) return `http://127.0.0.1:${env.FLOWPILOT_RUNNER_PORT}`;
  return "http://127.0.0.1:4317";
}
