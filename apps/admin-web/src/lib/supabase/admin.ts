import { createRuntimeSupabaseAdminClient } from "@/lib/supabase/runtime-config.server";

export function createSupabaseAdminClient() {
  return createRuntimeSupabaseAdminClient();
}
