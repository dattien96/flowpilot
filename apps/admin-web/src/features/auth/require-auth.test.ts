import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const { getBrowserSupabaseClient, getSession, loadSupabaseRuntimeStatus } = vi.hoisted(() => ({
  getBrowserSupabaseClient: vi.fn(),
  getSession: vi.fn(),
  loadSupabaseRuntimeStatus: vi.fn(),
}));

vi.mock("@/data/supabase/client", () => ({
  getBrowserSupabaseClient,
}));

vi.mock("@/lib/supabase/runtime-config", () => ({
  loadSupabaseRuntimeStatus,
}));

describe("requireAuth", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.spyOn(console, "error").mockImplementation(() => {});
    getBrowserSupabaseClient.mockReset();
    getSession.mockReset();
    loadSupabaseRuntimeStatus.mockReset();
    getBrowserSupabaseClient.mockResolvedValue({
      auth: {
        getSession,
      },
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns the current session when Supabase has an authenticated session", async () => {
    loadSupabaseRuntimeStatus.mockResolvedValue(supabaseRuntimeStatus());
    getSession.mockResolvedValue({
      data: {
        session: {
          user: {
            email: "admin@flowpilot.local",
            id: "user-1",
          },
        },
      },
    });

    const { requireAuth } = await import("./require-auth");
    const session = await requireAuth();

    expect(session).toEqual({
      mode: "supabase",
      user: {
        email: "admin@flowpilot.local",
        id: "user-1",
      },
    });
  });

  it("redirects guests to /login when no session exists", async () => {
    loadSupabaseRuntimeStatus.mockResolvedValue(supabaseRuntimeStatus());
    getSession.mockResolvedValue({
      data: {
        session: null,
      },
    });

    const { requireAuth } = await import("./require-auth");

    await expect(requireAuth()).rejects.toMatchObject({
      options: {
        to: "/login",
      },
    });
  });

  it("redirects guests to /login when the session lookup fails", async () => {
    loadSupabaseRuntimeStatus.mockResolvedValue(supabaseRuntimeStatus());
    getSession.mockRejectedValue(new Error("network down"));

    const { requireAuth } = await import("./require-auth");

    await expect(requireAuth()).rejects.toMatchObject({
      options: {
        to: "/login",
      },
    });
  });
});

function supabaseRuntimeStatus() {
  return {
    mode: "config",
    configured: true,
    apiUrl: "https://demo-ref.supabase.co",
    anonKey: "anon-key",
    edgeFunctionUrl: "https://demo-ref.supabase.co/functions/v1",
    hasServiceRoleKey: true,
    projectRef: "demo-ref",
    runnerReachable: true,
    envAvailable: false,
    savedConfigAvailable: true,
    edgeFunctionsReady: true,
    lastError: null,
  };
}
