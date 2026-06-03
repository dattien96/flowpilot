import { NextResponse } from "next/server";

import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";

export type SupabaseRuntimeMode = "config" | "env" | "demo";

export type SupabaseRuntimeStatus = {
  mode: SupabaseRuntimeMode;
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
  serviceRoleKey?: string;
};

export type SupabaseWorkspaceConfigPayload = {
  version?: number;
  apiUrl?: string;
  anonKey?: string;
  edgeFunctionUrl?: string;
  projectRef?: string;
  status?: string;
  updatedAt?: string;
  hasServiceRoleKey?: boolean;
  serviceRoleKey?: string;
};

export function localRunnerRequest(path: string, init?: RequestInit) {
  return fetch(new URL(path, getLocalRunnerBaseUrl()), {
    cache: "no-store",
    ...init,
  });
}

export async function readRunnerError(response: Response) {
  const text = await response.text();
  if (!text) {
    return `Runner request failed with status ${response.status}.`;
  }

  try {
    const payload = JSON.parse(text) as { error?: string };
    return payload.error || text;
  } catch {
    return text;
  }
}

export function browserSafeStatus(status: SupabaseRuntimeStatus) {
  const { serviceRoleKey: _serviceRoleKey, ...safeStatus } = status;
  return safeStatus;
}

export async function resolveSupabaseRuntimeStatus(options?: {
  includeServiceRoleKey?: boolean;
}): Promise<SupabaseRuntimeStatus> {
  let runnerReachable = true;
  let runnerError: string | null = null;
  const includeSecret = options?.includeServiceRoleKey ? "?includeSecret=1" : "";

  try {
    const response = await localRunnerRequest(`/supabase-config${includeSecret}`);
    if (response.ok) {
      const payload = (await response.json()) as SupabaseWorkspaceConfigPayload;
      if (payload.apiUrl && payload.anonKey) {
        return {
          mode: "config",
          configured: true,
          apiUrl: payload.apiUrl,
          anonKey: payload.anonKey,
          edgeFunctionUrl: payload.edgeFunctionUrl ?? null,
          hasServiceRoleKey: Boolean(payload.hasServiceRoleKey),
          projectRef: payload.projectRef ?? deriveProjectRef(payload.apiUrl),
          runnerReachable: true,
          envAvailable: hasCoreEnv(),
          savedConfigAvailable: true,
          edgeFunctionsReady: Boolean(payload.edgeFunctionUrl),
          lastError: null,
          serviceRoleKey: options?.includeServiceRoleKey
            ? payload.serviceRoleKey
            : undefined,
        };
      }
      runnerError = "Saved Supabase config is incomplete.";
    } else if (response.status !== 404) {
      runnerError = await readRunnerError(response);
    }
  } catch (error) {
    runnerReachable = false;
    runnerError =
      error instanceof Error ? error.message : "Unable to reach local runner.";
  }

  const envStatus = resolveEnvFallbackStatus();
  return {
    ...envStatus,
    runnerReachable,
    savedConfigAvailable: false,
    lastError: runnerError,
  };
}

export async function proxyRunnerJSON(
  path: string,
  init?: RequestInit,
): Promise<Response> {
  try {
    const response = await localRunnerRequest(path, init);
    const body = response.status === 204 ? null : await response.json().catch(() => null);
    if (!response.ok) {
      return NextResponse.json(
        { error: body?.error ?? `Runner request failed with status ${response.status}.` },
        { status: response.status },
      );
    }
    return NextResponse.json(body ?? {});
  } catch (error) {
    return NextResponse.json(
      {
        error: error instanceof Error ? error.message : "Unable to reach local runner.",
      },
      { status: 400 },
    );
  }
}

function resolveEnvFallbackStatus(): SupabaseRuntimeStatus {
  const envAvailable = hasCoreEnv();
  const apiUrl = process.env.SUPABASE_API_URL ?? null;
  const anonKey = process.env.SUPABASE_API_KEY ?? null;
  const edgeFunctionUrl = process.env.SUPABASE_API_EDGE_FUNCTION_URL ?? null;
  const serviceRoleKey =
    process.env.SUPABASE_SERVICE_ROLE_KEY ??
    process.env.SUPABASE_API_SERVICE_ROLE_KEY ??
    null;

  if (envAvailable) {
    return {
      mode: "env",
      configured: true,
      apiUrl,
      anonKey,
      edgeFunctionUrl,
      hasServiceRoleKey: Boolean(serviceRoleKey),
      projectRef: apiUrl ? deriveProjectRef(apiUrl) : null,
      runnerReachable: true,
      envAvailable: true,
      savedConfigAvailable: false,
      edgeFunctionsReady: Boolean(edgeFunctionUrl),
      lastError: null,
      serviceRoleKey: serviceRoleKey ?? undefined,
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
    runnerReachable: true,
    envAvailable: false,
    savedConfigAvailable: false,
    edgeFunctionsReady: false,
    lastError: null,
  };
}

function hasCoreEnv() {
  return Boolean(process.env.SUPABASE_API_URL && process.env.SUPABASE_API_KEY);
}

function deriveProjectRef(apiUrl: string) {
  try {
    const host = new URL(apiUrl).host.toLowerCase();
    return host.replace(/\.supabase\.co$/, "").split(".")[0] || null;
  } catch {
    return null;
  }
}
