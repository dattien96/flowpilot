import { createClient } from "@supabase/supabase-js";

import { getSupabaseAnonKey, getSupabaseUrl, hasSupabaseEnv } from "@/lib/env/app-env";

export {
  createSupabaseBrowserClient,
  supabase,
} from "@/data/supabase/client";

function createDemoSupabaseClient() {
  return {
    auth: {
      async getSession() {
        return { data: { session: null }, error: null };
      },
      async getUser() {
        return { data: { user: null }, error: null };
      },
      async signInWithPassword() {
        return {
          data: { session: null, user: null },
          error: new Error("Supabase env is not configured."),
        };
      },
      async signOut() {
        return { error: null };
      },
      onAuthStateChange() {
        return {
          data: {
            subscription: {
              unsubscribe() {},
            },
          },
        };
      },
    },
  };
}

export function createSupabaseServerClient() {
  return hasSupabaseEnv()
    ? createClient(getSupabaseUrl(), getSupabaseAnonKey())
    : (createDemoSupabaseClient() as never);
}
