type EnvSource = Record<string, string | undefined>;

function currentEnv(): EnvSource {
  return (import.meta as ImportMeta & { env?: EnvSource }).env ?? {};
}

function readRequiredEnv(name: "VITE_SUPABASE_URL" | "VITE_SUPABASE_ANON_KEY") {
  const value = currentEnv()[name];

  if (!value) {
    throw new Error(`Missing required environment variable: ${name}`);
  }

  return value;
}

export function hasSupabaseEnv() {
  const env = currentEnv();
  return Boolean(env.VITE_SUPABASE_URL && env.VITE_SUPABASE_ANON_KEY);
}

export function getSupabaseUrl() {
  return readRequiredEnv("VITE_SUPABASE_URL");
}

export function getSupabaseAnonKey() {
  return readRequiredEnv("VITE_SUPABASE_ANON_KEY");
}

export function getLocalRunnerBaseUrl() {
  return currentEnv().VITE_LOCAL_RUNNER_URL ?? "http://localhost:9100";
}
