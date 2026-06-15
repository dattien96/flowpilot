import { createClient, type SupabaseClient } from "@supabase/supabase-js";
import { desktopBridgeFetch } from "./desktopBridgeHttp";

type PersistedAuthSession = {
  clientKey: string;
  accessToken: string;
  refreshToken: string;
  userId: string;
  email?: string | null;
};

const AUTH_SESSION_STORAGE_KEY = "flowpilot.desktop.supabase-auth-session";

async function loadBrowserPersistedAuthSession(): Promise<PersistedAuthSession | null> {
  if (typeof window === "undefined" || !window.localStorage) {
    return null;
  }
  try {
    const raw = window.localStorage.getItem(AUTH_SESSION_STORAGE_KEY);
    return raw ? (JSON.parse(raw) as PersistedAuthSession) : null;
  } catch {
    return null;
  }
}

async function loadPersistedAuthSession(
  clientKey: string,
): Promise<PersistedAuthSession | null> {
  const candidate = globalThis as typeof globalThis & {
    window?: {
      flowpilot?: {
        loadAuthSession(): Promise<PersistedAuthSession | null>;
      };
    };
  };
  const persisted = candidate.window?.flowpilot?.loadAuthSession
    ? await candidate.window.flowpilot.loadAuthSession()
    : await loadBrowserPersistedAuthSession();
  if (!persisted || persisted.clientKey !== clientKey) {
    return null;
  }
  return persisted;
}

function clientKeyFor(apiUrl: string, anonKey: string) {
  return `${apiUrl}::${anonKey}`;
}

export async function createDesktopAdminSupabaseClient(
  apiUrl: string,
  anonKey: string,
): Promise<SupabaseClient> {
  const persisted = await loadPersistedAuthSession(clientKeyFor(apiUrl, anonKey));
  if (!persisted?.accessToken) {
    return createClient(apiUrl, anonKey, {
      global: { fetch: desktopBridgeFetch },
    });
  }
  return createClient(apiUrl, anonKey, {
    accessToken: async () => persisted.accessToken,
    global: { fetch: desktopBridgeFetch },
  });
}
