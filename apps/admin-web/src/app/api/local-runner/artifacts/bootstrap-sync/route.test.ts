import { beforeEach, describe, expect, it, vi } from "vitest";

import { POST } from "./route";

const mocks = vi.hoisted(() => ({
  createSupabaseAdminClient: vi.fn(),
  triggerArtifactSyncBootstrap: vi.fn(),
  storageGatewayInstance: {},
}));

vi.mock("@/lib/supabase/admin", () => ({
  createSupabaseAdminClient: mocks.createSupabaseAdminClient,
}));

vi.mock("@/features/artifacts/artifact-auto-sync", () => ({
  triggerArtifactSyncBootstrap: mocks.triggerArtifactSyncBootstrap,
}));

vi.mock("@/data/repository/supabase/supabase-artifact-storage-connection-gateway", () => ({
  SupabaseArtifactStorageConnectionGateway: vi
    .fn()
    .mockImplementation(() => mocks.storageGatewayInstance),
}));

describe("artifact bootstrap sync route", () => {
  beforeEach(() => {
    mocks.createSupabaseAdminClient.mockReset();
    mocks.triggerArtifactSyncBootstrap.mockReset();
  });

  it("starts the existing bootstrap flow", async () => {
    const supabase = {};
    mocks.createSupabaseAdminClient.mockReturnValue(supabase);

    const response = await POST();

    expect(mocks.triggerArtifactSyncBootstrap).toHaveBeenCalledWith({
      supabase,
      artifactStorageConnectionGateway: mocks.storageGatewayInstance,
    });
    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ started: true });
  });

  it("returns a 400 when bootstrap startup fails", async () => {
    mocks.createSupabaseAdminClient.mockImplementation(() => {
      throw new Error("runner unavailable");
    });

    const response = await POST();

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error: "runner unavailable",
    });
  });
});
