import { createClient } from "@supabase/supabase-js";

import {
  getSupabaseAnonKey,
  getSupabaseUrl,
  hasSupabaseEnv,
  hasSupabaseServiceEnv,
  getSupabaseServiceRoleKey,
} from "@/lib/env/browser-env";

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
          data: { user: null, session: null },
          error: new Error("Supabase is not configured for demo mode."),
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

export function createSupabaseBrowserClient() {
  if (!hasSupabaseEnv()) {
    return createDemoSupabaseClient() as never;
  }

  return createClient(getSupabaseUrl(), getSupabaseAnonKey());
}

export function createSupabaseServerClient() {
  if (!hasSupabaseServiceEnv()) {
    return createDemoSupabaseClient() as never;
  }

  return createClient(getSupabaseUrl(), getSupabaseServiceRoleKey());
}

export const createSupabaseServiceClient = createSupabaseServerClient;
export const supabase = createSupabaseBrowserClient();
