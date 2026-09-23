import test from "node:test";
import assert from "node:assert/strict";

import { DesktopSupabaseAuthRepository } from "../../apps/desktop-flowpilot/src/auth/desktopSupabaseAuthRepository";
import type {
  HttpClient,
  RuntimeConfigRepository,
  SupabaseConfigInput,
  SupabaseConfigValidation,
  SupabaseRuntimeStatus,
  SupabaseSchemaApplyInput,
  SupabaseSchemaApplyResult,
} from "../../packages/flowpilot-client-core/src";

const runtimeStatus: SupabaseRuntimeStatus = {
  mode: "config",
  configured: true,
  apiUrl: "https://proj.supabase.co",
  anonKey: "anon",
  edgeFunctionUrl: "https://proj.supabase.co/functions/v1",
  hasServiceRoleKey: true,
  projectRef: "proj",
  runnerReachable: true,
  envAvailable: false,
  savedConfigAvailable: true,
  edgeFunctionsReady: true,
  lastError: null,
};

class FakeRuntimeConfigRepository implements RuntimeConfigRepository {
  loadSupabaseRuntimeStatus(): Promise<SupabaseRuntimeStatus> {
    return Promise.resolve(runtimeStatus);
  }

  validateSupabaseConfig(_input: SupabaseConfigInput): Promise<SupabaseConfigValidation> {
    throw new Error("Not implemented in test.");
  }

  applySupabaseMigrations(_input: SupabaseSchemaApplyInput): Promise<SupabaseSchemaApplyResult> {
    throw new Error("Not implemented in test.");
  }

  saveSupabaseConfig(_input: SupabaseConfigInput): Promise<SupabaseRuntimeStatus> {
    throw new Error("Not implemented in test.");
  }
}

class FakeHttpClient implements HttpClient {
  constructor(
    private readonly handler: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>,
  ) {}

  request(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    return this.handler(input, init);
  }
}

type PersistedAuthSession = {
  clientKey: string;
  accessToken: string;
  refreshToken: string;
  userId: string;
  email?: string | null;
};

test("DesktopSupabaseAuthRepository keeps fallback session visible to bootstrap after runner login", async () => {
  const originalWindow = (globalThis as { window?: unknown }).window;
  let persistedSession: PersistedAuthSession | null = null;
  (globalThis as { window?: unknown }).window = {
    flowpilot: {
      loadAuthSession: async () => persistedSession,
      saveAuthSession: async (payload: PersistedAuthSession) => {
        persistedSession = payload;
        return { ok: true };
      },
      clearAuthSession: async () => {
        persistedSession = null;
        return { ok: true };
      },
    },
  };

  try {
    const repository = new DesktopSupabaseAuthRepository(
      new FakeRuntimeConfigRepository(),
      new FakeHttpClient(async () =>
        new Response(
          JSON.stringify({
            accessToken: "access-token",
            refreshToken: "refresh-token",
            userId: "user-1",
            email: "user@example.com",
          }),
          { status: 200 },
        )),
      "http://127.0.0.1:4317",
    );

    const authCalls: {
      signInWithPassword: Array<{ email: string; password: string }>;
      setSession: Array<{ access_token: string; refresh_token: string }>;
      getSession: number;
    } = {
      signInWithPassword: [],
      setSession: [],
      getSession: 0,
    };

    const supabaseStub = {
      auth: {
        signInWithPassword: async (credentials: { email: string; password: string }) => {
          authCalls.signInWithPassword.push(credentials);
          throw new TypeError("Failed to fetch");
        },
        setSession: async (tokens: { access_token: string; refresh_token: string }) => {
          authCalls.setSession.push(tokens);
          return { data: { session: null }, error: null };
        },
        getSession: async () => {
          authCalls.getSession += 1;
          return { data: { session: null }, error: null };
        },
      },
    };

    const repositoryState = repository as unknown as {
      client: typeof supabaseStub | null;
      clientKey: string | null;
    };
    repositoryState.client = supabaseStub;
    repositoryState.clientKey = `${runtimeStatus.apiUrl ?? ""}::${runtimeStatus.anonKey ?? ""}`;

    const loginSession = await repository.loginWithPassword("user@example.com", "secret");
    const restartedRepository = new DesktopSupabaseAuthRepository(
      new FakeRuntimeConfigRepository(),
      new FakeHttpClient(async () => {
        throw new Error("Runner should not be called when persisted auth session can be restored.");
      }),
      "http://127.0.0.1:4317",
    );
    const restartedSupabaseStub = {
      auth: {
        getSession: async () => {
          authCalls.getSession += 1;
          return { data: { session: null }, error: null };
        },
        setSession: async (tokens: { access_token: string; refresh_token: string }) => {
          authCalls.setSession.push(tokens);
          return {
            data: {
              session: {
                access_token: tokens.access_token,
                refresh_token: tokens.refresh_token,
                user: { id: "user-1", email: "user@example.com" },
              },
            },
            error: null,
          };
        },
      },
    };
    const restartedRepositoryState = restartedRepository as unknown as {
      client: typeof restartedSupabaseStub | null;
      clientKey: string | null;
    };
    restartedRepositoryState.client = restartedSupabaseStub;
    restartedRepositoryState.clientKey = `${runtimeStatus.apiUrl ?? ""}::${runtimeStatus.anonKey ?? ""}`;

    const bootstrapSession = await restartedRepository.getSession();

    assert.deepEqual(loginSession, {
      userId: "user-1",
      email: "user@example.com",
    });
    assert.deepEqual(bootstrapSession, loginSession);
    assert.deepEqual(authCalls.signInWithPassword, [
      { email: "user@example.com", password: "secret" },
    ]);
    assert.deepEqual(authCalls.setSession, [
      { access_token: "access-token", refresh_token: "refresh-token" },
      { access_token: "access-token", refresh_token: "refresh-token" },
    ]);
    assert.equal(authCalls.getSession, 1);
    assert.deepEqual(persistedSession, {
      clientKey: "https://proj.supabase.co::anon",
      accessToken: "access-token",
      refreshToken: "refresh-token",
      userId: "user-1",
      email: "user@example.com",
    });
  } finally {
    (globalThis as { window?: unknown }).window = originalWindow;
  }
});

// BUG-376 (CP-81 "random logout"): after a successful restore, setSession may
// ROTATE the refresh token (Supabase refresh tokens are single-use). The
// repository persisted the request `payload` — i.e. the OLD, now-dead tokens —
// so the next cold start restored with a stale refresh token and the session
// silently degraded to logged-out. The persisted session must carry the
// tokens from the setSession response.
test("DesktopSupabaseAuthRepository persists rotated tokens after restore, not the stale payload", async () => {
  const originalWindow = (globalThis as { window?: unknown }).window;
  let persistedSession: PersistedAuthSession | null = {
    clientKey: "https://proj.supabase.co::anon",
    accessToken: "stale-access-token",
    refreshToken: "stale-refresh-token",
    userId: "user-1",
    email: "user@example.com",
  };
  (globalThis as { window?: unknown }).window = {
    flowpilot: {
      loadAuthSession: async () => persistedSession,
      saveAuthSession: async (payload: PersistedAuthSession) => {
        persistedSession = payload;
        return { ok: true };
      },
      clearAuthSession: async () => {
        persistedSession = null;
        return { ok: true };
      },
    },
  };
  try {
    const repository = new DesktopSupabaseAuthRepository(
      new FakeRuntimeConfigRepository(),
      new FakeHttpClient(async () => {
        throw new Error("Runner should not be called when persisted auth session can be restored.");
      }),
      "http://127.0.0.1:4317",
    );
    const supabaseStub = {
      auth: {
        getSession: async () => ({ data: { session: null }, error: null }),
        setSession: async (_tokens: { access_token: string; refresh_token: string }) => ({
          // Supabase rotation: the response carries NEW tokens.
          data: {
            session: {
              access_token: "rotated-access-token",
              refresh_token: "rotated-refresh-token",
              user: { id: "user-1", email: "user@example.com" },
            },
          },
          error: null,
        }),
      },
    };
    const repositoryState = repository as unknown as {
      client: typeof supabaseStub | null;
      clientKey: string | null;
    };
    repositoryState.client = supabaseStub;
    repositoryState.clientKey = "https://proj.supabase.co::anon";

    const session = await repository.getSession();
    assert.deepEqual(session, { userId: "user-1", email: "user@example.com" });
    assert.equal(
      persistedSession?.refreshToken,
      "rotated-refresh-token",
      "persisted session must carry the rotated refresh token, not the consumed one",
    );
    assert.equal(persistedSession?.accessToken, "rotated-access-token");
  } finally {
    (globalThis as { window?: unknown }).window = originalWindow;
  }
});
