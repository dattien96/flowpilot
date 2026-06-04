import { createClient } from "@supabase/supabase-js";

import {
  getCachedBrowserSupabaseConfig,
  invalidateSupabaseRuntimeStatus,
  loadBrowserSupabaseConfig,
} from "@/lib/supabase/runtime-config";

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
    channel() {
      return {
        on() {
          return this;
        },
        subscribe() {
          return this;
        },
      };
    },
    removeChannel() {
      return "ok";
    },
  };
}

export function createSupabaseBrowserClient() {
  const config = getCachedBrowserSupabaseConfig();
  return config
    ? createClient(config.apiUrl, config.anonKey)
    : (createDemoSupabaseClient() as never);
}

let browserClient: ReturnType<typeof createSupabaseBrowserClient> | null = null;

export async function getBrowserSupabaseClient() {
  if (browserClient) {
    return browserClient;
  }

  const config = await loadBrowserSupabaseConfig();
  browserClient = config
    ? createClient(config.apiUrl, config.anonKey)
    : (createDemoSupabaseClient() as never);
  return browserClient;
}

export function resetBrowserSupabaseClient() {
  browserClient = null;
  invalidateSupabaseRuntimeStatus();
}

export const supabase = createDemoSupabaseClient() as never;
