import { afterEach, describe, expect, it, vi } from "vitest";

import { SupabaseArtifactStorageConnectionGateway } from "./supabase-artifact-storage-connection-gateway";

function createQueryResult(result: { data: unknown; error: { message: string } | null }) {
  const maybeSingle = vi.fn(async () => result);
  const eqProvider = vi.fn(() => ({ maybeSingle }));
  const eqProject = vi.fn(() => ({ eq: eqProvider, maybeSingle }));
  const select = vi.fn(() => ({ eq: eqProject, maybeSingle }));
  return {
    select,
    eqProject,
    eqProvider,
    maybeSingle,
  };
}

function createSupabaseStub(options?: {
  artifactRunResult?: { data: { id: string; project_id: string } | null; error: { message: string } | null };
  connectionResult?: { data: { folder_id: string } | null; error: { message: string } | null };
  replicaUpsertResult?: { error: { message: string } | null };
}) {
  const artifactRunResult = options?.artifactRunResult ?? {
    data: { id: "artifact-1", project_id: "project-alpha" },
    error: null,
  };
  const connectionResult = options?.connectionResult ?? {
    data: { folder_id: "folder-1" },
    error: null,
  };
  const replicaUpsertResult = options?.replicaUpsertResult ?? { error: null };

  const artifactRunMaybeSingle = vi.fn(async () => artifactRunResult);
  const artifactRunSelect = vi.fn(() => ({ maybeSingle: artifactRunMaybeSingle }));
  const artifactRunEq = vi.fn(() => ({ select: artifactRunSelect }));
  const connectionQuery = createQueryResult(connectionResult);
  const replicaUpsert = vi.fn(async () => replicaUpsertResult);
  const artifactRunUpdate = vi.fn(() => ({
    eq: artifactRunEq,
  }));
  const from = vi.fn((table: string) => {
    switch (table) {
      case "artifact_runs":
        return {
          update: artifactRunUpdate,
        };
      case "artifact_storage_connections":
        return {
          select: connectionQuery.select,
        };
      case "artifact_run_replicas":
        return {
          upsert: replicaUpsert,
        };
      default:
        throw new Error(`Unexpected table ${table}`);
    }
  });

  return {
    client: { from },
    from,
    artifactRunUpdate,
    artifactRunSelect,
    artifactRunMaybeSingle,
    connectionQuery,
    replicaUpsert,
  };
}

describe("SupabaseArtifactStorageConnectionGateway", () => {
  afterEach(() => {
    delete process.env.SUPABASE_API_URL;
  });

  it("updates artifact sync metadata when the artifact run exists", async () => {
    process.env.SUPABASE_API_URL = "https://example.supabase.co";
    const stub = createSupabaseStub();
    const gateway = new SupabaseArtifactStorageConnectionGateway(stub.client as never);

    await gateway.updateArtifactRunStorageMetadata({
      artifactRunId: "artifact-1",
      storageProvider: "supabase",
      remotePath: "projects/project-alpha/runs/run-1/steps/plan/Plan.md",
      remoteObjectId: "object-1",
      syncStatus: "synced",
    });

    expect(stub.from).toHaveBeenCalledWith("artifact_runs");
    expect(stub.artifactRunUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        storage_provider: "supabase",
        remote_path: "projects/project-alpha/runs/run-1/steps/plan/Plan.md",
        remote_object_id: "object-1",
        sync_status: "synced",
      }),
    );
    expect(stub.replicaUpsert).toHaveBeenCalledWith(
      expect.objectContaining({
        artifact_run_id: "artifact-1",
        project_id: "project-alpha",
        provider: "supabase",
        storage_scope_key: "supabase:https://example.supabase.co:flowpilot-artifacts",
      }),
      { onConflict: "artifact_run_id,provider" },
    );
  });

  it("uses the connected google drive folder to derive storage scope metadata", async () => {
    const stub = createSupabaseStub();
    const gateway = new SupabaseArtifactStorageConnectionGateway(stub.client as never);

    await gateway.updateArtifactRunStorageMetadata({
      artifactRunId: "artifact-1",
      storageProvider: "google_drive",
      remotePath: "projects/project-alpha/runs/run-1/steps/plan/Plan.md",
      remoteObjectId: "drive-object-1",
      syncStatus: "synced",
    });

    expect(stub.from).toHaveBeenCalledWith("artifact_storage_connections");
    expect(stub.replicaUpsert).toHaveBeenCalledWith(
      expect.objectContaining({
        provider: "google_drive",
        storage_scope_key: "gdrive:folder-1",
      }),
      { onConflict: "artifact_run_id,provider" },
    );
  });

  it("throws when shared sync state is missing", async () => {
    const stub = createSupabaseStub({
      artifactRunResult: {
        data: null,
        error: null,
      },
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
