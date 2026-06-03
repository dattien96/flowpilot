import { createClient } from "@supabase/supabase-js";

import { resolveSupabaseRuntimeStatus } from "@/app/api/runtime/supabase-config/_shared";

export async function resolveSupabaseRuntimeConfig() {
  return resolveSupabaseRuntimeStatus({ includeServiceRoleKey: true });
}

export async function hasSupabaseRuntimeConfigOrEnvFallback() {
  const status = await resolveSupabaseRuntimeStatus();
  return status.configured;
}

export async function createRuntimeSupabaseAnonClient() {
  const status = await resolveSupabaseRuntimeStatus();
  if (!status.configured || !status.apiUrl || !status.anonKey) {
    throw new Error("Supabase runtime config is not configured.");
  }
  return createClient(status.apiUrl, status.anonKey);
}

export async function createRuntimeSupabaseAdminClient() {
  const status = await resolveSupabaseRuntimeConfig();
  if (!status.configured || !status.apiUrl) {
    throw new Error("Supabase runtime config is not configured.");
  }
  if (!status.serviceRoleKey) {
    throw new Error("Supabase service role key is not configured.");
  }
  return createClient(status.apiUrl, status.serviceRoleKey);
}
