import { createRuntimeSupabaseAnonClient } from "@/lib/supabase/runtime-config.server";

export {
  createSupabaseBrowserClient,
  getBrowserSupabaseClient,
  resetBrowserSupabaseClient,
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
  return createRuntimeSupabaseAnonClient().catch(
    () => createDemoSupabaseClient() as never,
  );
}
