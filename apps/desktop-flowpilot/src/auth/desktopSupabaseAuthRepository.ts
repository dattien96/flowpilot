import { createClient, type SupabaseClient } from "@supabase/supabase-js";
import type { AuthRepository, AuthSession } from "@flowpilot/client-core";
import type { RuntimeConfigRepository, SupabaseRuntimeStatus } from "@flowpilot/client-core";
import type { HttpClient } from "@flowpilot/client-core";

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
    try {
      const { data, error } = await supabase.auth.getSession();
      if (error) {
        throw error;
      }
      return mapAuthSession(data.session);
    } catch (error) {
      if (isFetchFailure(error)) {
        return this.fallbackSession;
      }
      throw error;
    }
  }

  async loginWithPassword(email: string, password: string): Promise<AuthSession> {
    const supabase = await this.getClient(true);
    if (!supabase) {
      throw new Error("Supabase database is not configured. Please configure this PC first.");
    }
    let sessionData: { session: { user?: { id: string; email?: string | null } | null } | null };
    try {
      const response = await supabase.auth.signInWithPassword({ email, password });
      if (response.error) {
        throw response.error;
      }
      sessionData = response.data;
    } catch (error) {
      if (isFetchFailure(error)) {
        sessionData = await this.loginThroughRunner(email, password);
      } else {
        throw error;
      }
    }
    const session = mapAuthSession(sessionData.session);
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
        return;
      }
      throw error;
    }
    this.fallbackSession = null;
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
    return {
      session: {
        user: {
          id: payload.userId,
          email: payload.email ?? null,
        },
      },
    };
  }
}
