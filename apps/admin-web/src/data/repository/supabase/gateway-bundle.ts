import { createSupabaseServerClient } from "@/data/datasource/supabase/client";

export async function assertSupabaseBundleReady() {
  return createSupabaseServerClient();
}
