import { createClient } from "@supabase/supabase-js";

import {
  getSupabaseAnonKey,
  getSupabaseUrl,
  hasSupabaseEnv,
} from "@/lib/env/browser-env";

function createDemoSupabaseClient() {
  return {
    auth: {
      async getSession() {
        return { data: { session: null }, error: null };
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
              unsubscribe() { },
            },
          },
        };
      },
    },
  };
}

export function createSupabaseBrowserClient() {
  return hasSupabaseEnv()
    ? createClient(getSupabaseUrl(), getSupabaseAnonKey())
    : (createDemoSupabaseClient() as never);
}

export const supabase = createSupabaseBrowserClient();
