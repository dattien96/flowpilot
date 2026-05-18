import { supabase } from "@/data/supabase/client";
import {
  getSupabaseAnonKey,
  getSupabaseEdgeFunctionUrl,
  hasSupabaseEnv,
} from "@/lib/env/browser-env";

type EdgeFunctionMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

interface InvokeEdgeFunctionOptions {
  headers?: HeadersInit;
  method?: EdgeFunctionMethod;
  signal?: AbortSignal;
}

async function buildEdgeFunctionHeaders(headers?: HeadersInit) {
  const mergedHeaders = new Headers(headers);
  const apiKey = getSupabaseAnonKey();

  if (!mergedHeaders.has("Content-Type")) {
    mergedHeaders.set("Content-Type", "application/json");
  }

  if (!mergedHeaders.has("apikey")) {
    mergedHeaders.set("apikey", apiKey);
  }

  if (!mergedHeaders.has("Authorization")) {
    const accessToken = hasSupabaseEnv()
      ? (await supabase.auth.getSession()).data.session?.access_token ?? null
      : null;

    mergedHeaders.set("Authorization", `Bearer ${accessToken ?? apiKey}`);
  }

  return mergedHeaders;
}

export async function invokeSupabaseEdgeFunction<TResponse>(
  functionName: string,
  payload?: unknown,
  options?: InvokeEdgeFunctionOptions,
) {
  const method = options?.method ?? "POST";
  const baseUrl = getSupabaseEdgeFunctionUrl().replace(/\/+$/, "");
  const response = await fetch(`${baseUrl}/${functionName}`, {
    method,
    headers: await buildEdgeFunctionHeaders(options?.headers),
    signal: options?.signal,
    body: payload === undefined ? undefined : JSON.stringify(payload),
  });

  if (!response.ok) {
    const message = await response.text();
    throw new Error(
      `Supabase Edge Function ${functionName} failed (${response.status}): ${message}`,
    );
  }

  if (response.status === 204) {
    return null as TResponse;
  }

  return (await response.json()) as TResponse;
}
