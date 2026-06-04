import {
  getSupabaseAnonKey,
  getSupabaseEdgeFunctionUrl,
  getSupabaseUrl,
  hasSupabaseEnv,
} from "@/lib/env/browser-env";

export type SupabaseRuntimeStatus = {
  mode: "config" | "env" | "demo";
  configured: boolean;
  apiUrl: string | null;
  anonKey: string | null;
  edgeFunctionUrl: string | null;
  hasServiceRoleKey: boolean;
  projectRef: string | null;
  runnerReachable: boolean;
  envAvailable: boolean;
  savedConfigAvailable: boolean;
  edgeFunctionsReady: boolean;
  lastError: string | null;
};

let cachedStatus: SupabaseRuntimeStatus | null = null;
let statusPromise: Promise<SupabaseRuntimeStatus> | null = null;

export async function loadSupabaseRuntimeStatus() {
  if (cachedStatus) {
    return cachedStatus;
  }
  if (!statusPromise) {
    statusPromise = fetch("/api/runtime/supabase-config", { cache: "no-store" })
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(await response.text());
        }
        return (await response.json()) as SupabaseRuntimeStatus;
      })
      .catch((error) => fallbackBrowserRuntimeStatus(error))
      .finally(() => {
        statusPromise = null;
      });
  }
  cachedStatus = await statusPromise;
  return cachedStatus;
}

export function getCachedSupabaseRuntimeStatus() {
  return cachedStatus;
}

export function invalidateSupabaseRuntimeStatus() {
  cachedStatus = null;
  statusPromise = null;
}

export function hasSupabaseRuntimeConfigOrEnvFallbackSync() {
  if (cachedStatus) {
    return cachedStatus.configured;
  }
  return hasSupabaseEnv();
}

export function getCachedBrowserSupabaseConfig() {
  if (cachedStatus?.configured && cachedStatus.apiUrl && cachedStatus.anonKey) {
    return {
      apiUrl: cachedStatus.apiUrl,
      anonKey: cachedStatus.anonKey,
      edgeFunctionUrl: cachedStatus.edgeFunctionUrl,
    };
  }
  if (hasSupabaseEnv()) {
    return {
      apiUrl: getSupabaseUrl(),
      anonKey: getSupabaseAnonKey(),
      edgeFunctionUrl: safeGetEdgeFunctionUrl(),
    };
  }
  return null;
}

export async function loadBrowserSupabaseConfig() {
  const status = await loadSupabaseRuntimeStatus();
  if (!status.configured || !status.apiUrl || !status.anonKey) {
    return null;
  }
  return {
    apiUrl: status.apiUrl,
    anonKey: status.anonKey,
    edgeFunctionUrl: status.edgeFunctionUrl,
  };
}

function fallbackBrowserRuntimeStatus(error: unknown): SupabaseRuntimeStatus {
  if (hasSupabaseEnv()) {
    return {
      mode: "env",
      configured: true,
      apiUrl: getSupabaseUrl(),
      anonKey: getSupabaseAnonKey(),
      edgeFunctionUrl: safeGetEdgeFunctionUrl(),
      hasServiceRoleKey: false,
      projectRef: deriveProjectRef(getSupabaseUrl()),
      runnerReachable: false,
      envAvailable: true,
      savedConfigAvailable: false,
      edgeFunctionsReady: Boolean(safeGetEdgeFunctionUrl()),
      lastError: error instanceof Error ? error.message : "Runtime config API is unavailable.",
    };
  }
  return {
    mode: "demo",
    configured: false,
    apiUrl: null,
    anonKey: null,
    edgeFunctionUrl: null,
    hasServiceRoleKey: false,
    projectRef: null,
    runnerReachable: false,
    envAvailable: false,
    savedConfigAvailable: false,
    edgeFunctionsReady: false,
    lastError: error instanceof Error ? error.message : "Runtime config API is unavailable.",
  };
}

function safeGetEdgeFunctionUrl() {
  try {
    return getSupabaseEdgeFunctionUrl();
  } catch {
    return null;
  }
}

function deriveProjectRef(apiUrl: string) {
  try {
    const host = new URL(apiUrl).host.toLowerCase();
    return host.replace(/\.supabase\.co$/, "").split(".")[0] || null;
  } catch {
    return null;
  }
}
