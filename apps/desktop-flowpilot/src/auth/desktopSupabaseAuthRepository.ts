import { createClient, type SupabaseClient } from "@supabase/supabase-js";
import type { AuthRepository, AuthSession } from "@flowpilot/client-core";
import type { RuntimeConfigRepository, SupabaseRuntimeStatus } from "@flowpilot/client-core";
import type { HttpClient } from "@flowpilot/client-core";

type PersistedAuthSession = {
  clientKey: string;
  accessToken: string;
  refreshToken: string;
  userId: string;
  email?: string | null;
};

type FlowpilotBridge = {
  loadAuthSession(): Promise<PersistedAuthSession | null>;
  saveAuthSession(session: PersistedAuthSession): Promise<{ ok: boolean }>;
  clearAuthSession(): Promise<{ ok: boolean }>;
};

const AUTH_SESSION_STORAGE_KEY = "flowpilot.desktop.supabase-auth-session";

function mapAuthSession(session: {
  user?: { id: string; email?: string | null } | null;
} | null): AuthSession | null {
  if (!session?.user) {
    return null;
  }
  return {
    userId: session.user.id,
    email: session.user.email ?? null,
  };
}

function isFetchFailure(error: unknown) {
  if (error instanceof TypeError && /Failed to fetch/i.test(error.message)) {
    return true;
  }
  if (
    typeof error === "object" &&
    error !== null &&
    "message" in error &&
    typeof (error as { message: unknown }).message === "string"
  ) {
    return /Failed to fetch/i.test((error as { message: string }).message);
  }
  return false;
}

function toSupabaseReachabilityError() {
  return new Error(
    "Unable to reach the configured Supabase project. Check the Supabase URL, network access, and local configuration.",
  );
}

function mapPersistedAuthSession(session: PersistedAuthSession): AuthSession {
  return {
    userId: session.userId,
    email: session.email ?? null,
  };
}

function toPersistedAuthSession(
  session: {
    access_token?: string;
    refresh_token?: string;
    user?: { id: string; email?: string | null } | null;
  } | null,
  clientKey: string,
): PersistedAuthSession | null {
  if (!session?.access_token || !session.refresh_token || !session.user?.id) {
    return null;
  }
  return {
    clientKey,
    accessToken: session.access_token,
    refreshToken: session.refresh_token,
    userId: session.user.id,
    email: session.user.email ?? null,
  };
}

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

async function saveBrowserPersistedAuthSession(session: PersistedAuthSession): Promise<void> {
  if (typeof window === "undefined" || !window.localStorage) {
    return;
  }
  try {
    window.localStorage.setItem(AUTH_SESSION_STORAGE_KEY, JSON.stringify(session));
  } catch {
    // Ignore dev-tab storage failures and keep the in-memory session alive.
  }
}

async function clearBrowserPersistedAuthSession(): Promise<void> {
  if (typeof window === "undefined" || !window.localStorage) {
    return;
  }
  try {
    window.localStorage.removeItem(AUTH_SESSION_STORAGE_KEY);
  } catch {
    // Ignore dev-tab storage failures and keep the app responsive.
  }
}

function getFlowpilotBridge(): FlowpilotBridge | undefined {
  const candidate = globalThis as typeof globalThis & {
    window?: { flowpilot?: FlowpilotBridge };
  };
  return candidate.window?.flowpilot;
}

export class DesktopSupabaseAuthRepository implements AuthRepository {
  private client: SupabaseClient | null = null;
  private clientKey: string | null = null;
  private fallbackSession: AuthSession | null = null;

  constructor(
    private readonly runtimeConfigRepository: RuntimeConfigRepository,
    private readonly httpClient: HttpClient,
    private readonly runnerBaseUrl: string,
  ) {}

  async getSession(): Promise<AuthSession | null> {
    const supabase = await this.getClient();
    if (!supabase) return null;
    const clientKey = this.clientKey;
    try {
      const { data, error } = await supabase.auth.getSession();
      if (error) {
        throw error;
      }
      const session = mapAuthSession(data.session);
      if (session) {
        this.fallbackSession = session;
        const persistedSession = toPersistedAuthSession(data.session, clientKey ?? "");
        if (persistedSession) {
          await this.savePersistedAuthSession(persistedSession);
        }
        return session;
      }

      const restoredSession = await this.restorePersistedSession(supabase, clientKey);
      return restoredSession ?? this.fallbackSession;
    } catch (error) {
      if (isFetchFailure(error)) {
        return this.fallbackSession ?? (await this.loadPersistedSessionIdentity(clientKey));
      }
      throw error;
    }
  }

  async loginWithPassword(email: string, password: string): Promise<AuthSession> {
    const supabase = await this.getClient(true);
    if (!supabase) {
      throw new Error("Supabase database is not configured. Please configure this PC first.");
    }
    let session: AuthSession | null = null;
    try {
      const response = await supabase.auth.signInWithPassword({ email, password });
      if (response.error) {
        throw response.error;
      }
      session = mapAuthSession(response.data.session);
      const persistedSession = toPersistedAuthSession(
        response.data.session,
        this.clientKey ?? this.clientIdentityFromClient(supabase),
      );
      if (persistedSession) {
        await this.savePersistedAuthSession(persistedSession);
      }
    } catch (error) {
      if (isFetchFailure(error)) {
        const payload = await this.loginThroughRunner(email, password);
        session = await this.persistRunnerSession(supabase, payload);
      } else {
        throw error;
      }
    }
    if (!session) {
      throw new Error("Supabase returned no session.");
    }
    this.fallbackSession = session;
    return session;
  }

  async logout(): Promise<void> {
    const supabase = await this.getClient();
    if (!supabase) return;
    try {
      const { error } = await supabase.auth.signOut();
      if (error) {
        throw error;
      }
    } catch (error) {
      if (isFetchFailure(error)) {
        this.fallbackSession = null;
        await this.clearPersistedAuthSession();
        return;
      }
      throw error;
    }
    this.fallbackSession = null;
    await this.clearPersistedAuthSession();
  }

  resetSessionState() {
    this.client = null;
    this.clientKey = null;
    this.fallbackSession = null;
  }

  private async getClient(requireConfigured = false): Promise<SupabaseClient | null> {
    const runtimeStatus = await this.runtimeConfigRepository.loadSupabaseRuntimeStatus();
    if (!runtimeStatus.configured || !runtimeStatus.apiUrl || !runtimeStatus.anonKey) {
      if (requireConfigured) {
        throw new Error("Supabase database is not configured. Please configure this PC first.");
      }
      return null;
    }

    const nextKey = this.clientIdentity(runtimeStatus);
    if (!this.client || this.clientKey !== nextKey) {
      this.client = createClient(runtimeStatus.apiUrl, runtimeStatus.anonKey);
      this.clientKey = nextKey;
    }
    return this.client;
  }

  private clientIdentity(runtimeStatus: SupabaseRuntimeStatus) {
    return `${runtimeStatus.apiUrl ?? ""}::${runtimeStatus.anonKey ?? ""}`;
  }

  private async loginThroughRunner(
    email: string,
    password: string,
  ) {
    let response: Response;
    try {
      response = await this.httpClient.request(
        new URL("/supabase-auth/login", this.runnerBaseUrl),
        {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ email, password }),
        },
      );
    } catch (error) {
      if (isFetchFailure(error)) {
        throw new Error("Unable to reach the local runner for Supabase login fallback.");
      }
      throw error;
    }
    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || "Runner-side Supabase login failed.");
    }

    const payload = (await response.json()) as {
      accessToken: string;
      refreshToken: string;
      userId: string;
      email?: string | null;
    };
    return payload;
  }

  // Keep bootstrap-visible auth state even when direct renderer-side Supabase auth is unavailable.
  private async persistRunnerSession(
    supabase: SupabaseClient,
    payload: {
      accessToken: string;
      refreshToken: string;
      userId: string;
      email?: string | null;
    },
  ): Promise<AuthSession> {
    try {
      const { data, error } = await supabase.auth.setSession({
        access_token: payload.accessToken,
        refresh_token: payload.refreshToken,
      });
      if (error) {
        throw error;
      }
      const session = mapAuthSession(data.session);
      if (session) {
        await this.savePersistedAuthSession({
          ...payload,
          clientKey: this.clientKey ?? this.clientIdentityFromClient(supabase),
        });
        return session;
      }
    } catch {
      // Fall back to the runner-issued identity so the desktop bootstrap can continue.
    }

    await this.savePersistedAuthSession({
      ...payload,
      clientKey: this.clientKey ?? this.clientIdentityFromClient(supabase),
    });
    return {
      userId: payload.userId,
      email: payload.email ?? null,
    };
  }

  private clientIdentityFromClient(_supabase: SupabaseClient) {
    return this.clientKey ?? "";
  }

  private async restorePersistedSession(
    supabase: SupabaseClient,
    clientKey: string | null,
  ): Promise<AuthSession | null> {
    const persistedSession = await this.loadPersistedAuthSession(clientKey);
    if (!persistedSession) {
      return null;
    }
    const session = await this.persistRunnerSession(supabase, persistedSession);
    this.fallbackSession = session;
    return session;
  }

  private async loadPersistedSessionIdentity(clientKey: string | null): Promise<AuthSession | null> {
    const persistedSession = await this.loadPersistedAuthSession(clientKey);
    return persistedSession ? mapPersistedAuthSession(persistedSession) : null;
  }

  private async loadPersistedAuthSession(
    clientKey: string | null,
  ): Promise<PersistedAuthSession | null> {
    const bridge = getFlowpilotBridge();
    const persistedSession = bridge?.loadAuthSession
      ? await bridge.loadAuthSession()
      : await loadBrowserPersistedAuthSession();
    if (!persistedSession || (clientKey && persistedSession.clientKey !== clientKey)) {
      return null;
    }
    return persistedSession;
  }

  private async savePersistedAuthSession(session: PersistedAuthSession): Promise<void> {
    const bridge = getFlowpilotBridge();
    if (bridge?.saveAuthSession) {
      await bridge.saveAuthSession(session);
      return;
    }
    await saveBrowserPersistedAuthSession(session);
  }

  private async clearPersistedAuthSession(): Promise<void> {
    const bridge = getFlowpilotBridge();
    if (bridge?.clearAuthSession) {
      await bridge.clearAuthSession();
      return;
    }
    await clearBrowserPersistedAuthSession();
  }
}
