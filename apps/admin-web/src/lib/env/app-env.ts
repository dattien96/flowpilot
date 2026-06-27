import path from "node:path";
import fs from "node:fs";

export function hasSupabaseEnv() {
  return Boolean(
    process.env.SUPABASE_API_URL &&
      process.env.SUPABASE_API_KEY,
  );
}

export function getSupabaseUrl() {
  return getRequiredEnv("SUPABASE_API_URL");
}

export function getSupabaseAnonKey() {
  return getRequiredEnv("SUPABASE_API_KEY");
}

export function getSupabaseEdgeFunctionUrl() {
  return getRequiredEnv("SUPABASE_API_EDGE_FUNCTION_URL");
}

export function getLocalRunnerBaseUrl() {
  if (process.env.FLOWPILOT_RUNNER_URL) {
    return process.env.FLOWPILOT_RUNNER_URL;
  }
  if (process.env.FLOWPILOT_RUNNER_PORT) {
    return `http://127.0.0.1:${process.env.FLOWPILOT_RUNNER_PORT}`;
  }
  return "http://127.0.0.1:4317";
}

export function getRequiredEnv(name: string) {
  const value = process.env[name];

  if (!value) {
    throw new Error(`Missing required environment variable: ${name}`);
  }

  return value;
}

export function getWorkspaceRoot() {
  const workspace = process.env.FLOWPILOT_WORKSPACE;
  if (workspace) {
    return workspace;
  }

  let current = process.cwd();

  for (;;) {
    if (fs.existsSync(path.join(current, ".agents"))) {
      return current;
    }

    const parent = path.dirname(current);
    if (parent === current) {
      return process.cwd();
    }

    current = parent;
  }
}
