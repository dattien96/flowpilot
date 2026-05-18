type EnvSource = Record<string, string | undefined>;

function readEnv(source: EnvSource, keys: string[]) {
  for (const key of keys) {
    const value = source[key];
    if (typeof value === "string" && value.length > 0) {
      return value;
    }
  }

  return null;
}

function currentEnv() {
  const metaEnv = (import.meta as ImportMeta & { env?: EnvSource }).env ?? {};
  const processEnv = typeof process !== "undefined" ? process.env : {};

  return {
    metaEnv,
    processEnv,
  };
}

export function hasSupabaseEnv() {
  const { metaEnv } = currentEnv();
  return Boolean(
    readEnv(metaEnv, ["VITE_SUPABASE_URL", "SUPABASE_API_URL"]) &&
      readEnv(metaEnv, ["VITE_SUPABASE_ANON_KEY", "SUPABASE_API_KEY"]),
  );
}

export function getSupabaseUrl() {
  const { metaEnv, processEnv } = currentEnv();
  const value = readEnv(metaEnv, ["VITE_SUPABASE_URL", "SUPABASE_API_URL"]) ??
    readEnv(processEnv, ["SUPABASE_API_URL"]);

  if (!value) {
    throw new Error("Missing required Supabase URL environment variable.");
  }

  return value;
}

export function getSupabaseAnonKey() {
  const { metaEnv, processEnv } = currentEnv();
  const value = readEnv(metaEnv, ["VITE_SUPABASE_ANON_KEY", "SUPABASE_API_KEY"]) ??
    readEnv(processEnv, ["SUPABASE_API_KEY"]);

  if (!value) {
    throw new Error("Missing required Supabase anon key environment variable.");
  }

  return value;
}

export function getSupabaseServiceRoleKey() {
  const { metaEnv, processEnv } = currentEnv();
  const value = readEnv(metaEnv, ["SUPABASE_SERVICE_ROLE_KEY"]) ??
    readEnv(processEnv, ["SUPABASE_SERVICE_ROLE_KEY"]);

  if (!value) {
    throw new Error("Missing required Supabase service role environment variable.");
  }

  return value;
}

export function hasSupabaseServiceEnv() {
  const { metaEnv, processEnv } = currentEnv();
  return Boolean(
    readEnv(metaEnv, ["VITE_SUPABASE_URL", "SUPABASE_API_URL"]) ??
      readEnv(processEnv, ["SUPABASE_API_URL"]),
  ) &&
    Boolean(
      readEnv(metaEnv, ["SUPABASE_SERVICE_ROLE_KEY"]) ??
        readEnv(processEnv, ["SUPABASE_SERVICE_ROLE_KEY"]),
  );
}

export function getLocalRunnerBaseUrl() {
  const { metaEnv, processEnv } = currentEnv();
  return (
    readEnv(metaEnv, ["VITE_FLOWPILOT_RUNNER_URL", "FLOWPILOT_RUNNER_URL"]) ??
    readEnv(processEnv, ["FLOWPILOT_RUNNER_URL"]) ??
    "http://127.0.0.1:4317"
  );
}
