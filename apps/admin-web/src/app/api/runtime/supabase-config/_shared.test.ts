import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resolveSupabaseRuntimeStatus } from "./_shared";

const ORIGINAL_ENV = { ...process.env };

describe("resolveSupabaseRuntimeStatus", () => {
  beforeEach(() => {
    vi.resetModules();
    process.env = { ...ORIGINAL_ENV };
    delete process.env.SUPABASE_API_URL;
    delete process.env.SUPABASE_API_KEY;
    delete process.env.SUPABASE_API_EDGE_FUNCTION_URL;
    delete process.env.SUPABASE_SERVICE_ROLE_KEY;
    delete process.env.SUPABASE_API_SERVICE_ROLE_KEY;
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    process.env = { ...ORIGINAL_ENV };
  });

  it("prefers saved workspace config over env fallback", async () => {
    process.env.SUPABASE_API_URL = "https://env-ref.supabase.co";
    process.env.SUPABASE_API_KEY = "env-anon";
    process.env.SUPABASE_API_EDGE_FUNCTION_URL = "https://env-ref.supabase.co/functions/v1";
    process.env.SUPABASE_SERVICE_ROLE_KEY = "env-service-role";
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            apiUrl: "https://saved-ref.supabase.co",
            anonKey: "saved-anon",
            edgeFunctionUrl: "https://saved-ref.supabase.co/functions/v1",
            hasServiceRoleKey: true,
            projectRef: "saved-ref",
          }),
          { status: 200 },
        ),
      ),
    );

    const status = await resolveSupabaseRuntimeStatus();

    expect(status.mode).toBe("config");
    expect(status.apiUrl).toBe("https://saved-ref.supabase.co");
    expect(status.anonKey).toBe("saved-anon");
    expect(status.envAvailable).toBe(true);
    expect(status.savedConfigAvailable).toBe(true);
  });

  it("falls back to env when saved config is unavailable", async () => {
    process.env.SUPABASE_API_URL = "https://env-ref.supabase.co";
    process.env.SUPABASE_API_KEY = "env-anon";
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response("not found", { status: 404 })),
    );

    const status = await resolveSupabaseRuntimeStatus();

    expect(status.mode).toBe("env");
    expect(status.configured).toBe(true);
    expect(status.apiUrl).toBe("https://env-ref.supabase.co");
    expect(status.savedConfigAvailable).toBe(false);
  });

  it("uses demo mode when neither saved config nor env is available", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(new Response("not found", { status: 404 })),
    );

    const status = await resolveSupabaseRuntimeStatus();

    expect(status.mode).toBe("demo");
    expect(status.configured).toBe(false);
    expect(status.envAvailable).toBe(false);
  });
});
