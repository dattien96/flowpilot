import { describe, expect, it, vi } from "vitest";

import { SupabaseArtifactStorageConnectionGateway } from "./supabase-artifact-storage-connection-gateway";

function createSupabaseStub(result: { data: { id: string } | null; error: { message: string } | null }) {
  const maybeSingle = vi.fn(async () => result);
  const select = vi.fn(() => ({ maybeSingle }));
  const eq = vi.fn(() => ({ select }));
  const update = vi.fn(() => ({ eq }));
  const from = vi.fn(() => ({ update }));

  return {
    client: { from },
    from,
    update,
    eq,
    select,
    maybeSingle,
  };
}

describe("SupabaseArtifactStorageConnectionGateway", () => {
  it("updates artifact sync metadata when the artifact run exists", async () => {
    const stub = createSupabaseStub({
      data: { id: "artifact-1" },
      error: null,
    });
    const gateway = new SupabaseArtifactStorageConnectionGateway(stub.client as never);

    await gateway.updateArtifactRunStorageMetadata({
      artifactRunId: "artifact-1",
      storageProvider: "supabase",
      remotePath: "projects/project-alpha/runs/run-1/steps/plan/Plan.md",
      remoteObjectId: "object-1",
      syncStatus: "synced",
    });

    expect(stub.from).toHaveBeenCalledWith("artifact_runs");
    expect(stub.update).toHaveBeenCalledWith(
      expect.objectContaining({
        storage_provider: "supabase",
        remote_path: "projects/project-alpha/runs/run-1/steps/plan/Plan.md",
        remote_object_id: "object-1",
        sync_status: "synced",
      }),
    );
    expect(stub.eq).toHaveBeenCalledWith("id", "artifact-1");
  });

  it("throws when shared sync state is missing", async () => {
    const stub = createSupabaseStub({
      data: null,
      error: null,
    });
    const gateway = new SupabaseArtifactStorageConnectionGateway(stub.client as never);

    await expect(
      gateway.updateArtifactRunStorageMetadata({
        artifactRunId: "artifact-missing",
        syncStatus: "syncing",
      }),
    ).rejects.toThrow(/workflow-backed artifacts/i);
  });
});
