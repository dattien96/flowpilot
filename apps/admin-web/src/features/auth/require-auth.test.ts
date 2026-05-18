import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const getSession = vi.fn();

vi.mock("@/data/supabase/client", () => ({
  supabase: {
    auth: {
      getSession,
    },
  },
}));

vi.mock("@/lib/env/browser-env", () => ({
  hasSupabaseEnv: vi.fn(),
}));

describe("requireAuth", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.spyOn(console, "error").mockImplementation(() => {});
    getSession.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns the current session when Supabase has an authenticated session", async () => {
    const { hasSupabaseEnv } = await import("@/lib/env/browser-env");
    vi.mocked(hasSupabaseEnv).mockReturnValue(true);
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
    const { hasSupabaseEnv } = await import("@/lib/env/browser-env");
    vi.mocked(hasSupabaseEnv).mockReturnValue(true);
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
    const { hasSupabaseEnv } = await import("@/lib/env/browser-env");
    vi.mocked(hasSupabaseEnv).mockReturnValue(true);
    getSession.mockRejectedValue(new Error("network down"));

    const { requireAuth } = await import("./require-auth");

    await expect(requireAuth()).rejects.toMatchObject({
      options: {
        to: "/login",
      },
    });
  });
});
