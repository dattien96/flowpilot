import { describe, expect, it, vi } from "vitest";

const { loadSupabaseRuntimeStatus } = vi.hoisted(() => ({
  loadSupabaseRuntimeStatus: vi.fn(),
}));

vi.mock("@/lib/supabase/runtime-config", () => ({
  loadSupabaseRuntimeStatus,
}));

describe("browser createGatewayBundle", () => {
  it("returns a synchronous lazy bundle and resolves runtime only when a gateway method runs", async () => {
    loadSupabaseRuntimeStatus.mockResolvedValue({
      mode: "demo",
      configured: false,
      apiUrl: null,
      anonKey: null,
      edgeFunctionUrl: null,
      hasServiceRoleKey: false,
      projectRef: null,
      runnerReachable: true,
      envAvailable: false,
      savedConfigAvailable: false,
      edgeFunctionsReady: false,
      lastError: null,
    });

    const { createGatewayBundle } = await import("./browser-factory");
    const bundle = createGatewayBundle();

    expect(typeof bundle.projectGateway.listProjects).toBe("function");
    expect(loadSupabaseRuntimeStatus).not.toHaveBeenCalled();

    await expect(bundle.projectGateway.listProjects()).resolves.toEqual(
      expect.any(Array),
    );
    expect(loadSupabaseRuntimeStatus).toHaveBeenCalledTimes(1);
  });
});
