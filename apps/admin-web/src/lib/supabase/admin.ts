import { createClient } from "@supabase/supabase-js";

import { getRequiredEnv, getSupabaseUrl } from "@/lib/env/app-env";

export function createSupabaseAdminClient() {
  return createClient(
    getSupabaseUrl(),
    process.env.SUPABASE_SERVICE_ROLE_KEY ??
      process.env.SUPABASE_API_SERVICE_ROLE_KEY ??
      getRequiredEnv("SUPABASE_SERVICE_ROLE_KEY"),
  );
}
