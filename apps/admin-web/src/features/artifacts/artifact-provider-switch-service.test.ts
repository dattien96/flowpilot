import { beforeEach, describe, expect, it, vi } from "vitest";

import { switchProjectArtifactStorageProvider } from "./artifact-provider-switch-service";

const mocks = vi.hoisted(() => ({
  getArtifactStorageConnection: vi.fn(),
  syncArtifactThroughRunner: vi.fn(),
}));

vi.mock("@/features/artifacts/artifact-storage-connection", () => ({
  getArtifactStorageConnection: mocks.getArtifactStorageConnection,
}));

vi.mock("@/features/artifacts/artifact-sync-service", () => ({
  syncArtifactThroughRunner: mocks.syncArtifactThroughRunner,
}));

function createSupabaseStub(artifactRuns: Array<Record<string, unknown>>) {
  const projectUpdateEq = vi.fn(async () => ({ error: null }));
  const projectUpdate = vi.fn(() => ({ eq: projectUpdateEq }));
  const artifactRunsEq = vi.fn(async () => ({
    data: artifactRuns,
    error: null,
  }));
  const artifactRunsSelect = vi.fn(() => ({
    eq: artifactRunsEq,
  }));
  const projectSelectMaybeSingle = vi.fn(async () => ({
    data: { artifact_storage_preference: "google_drive" },
    error: null,
  }));
  const projectSelectEq = vi.fn(() => ({
    maybeSingle: projectSelectMaybeSingle,
  }));
  const projectSelect = vi.fn(() => ({
    eq: projectSelectEq,
  }));

  const from = vi.fn((table: string) => {
    switch (table) {
      case "artifact_runs":
        return {
          select: artifactRunsSelect,
        };
      case "projects":
        return {
          select: projectSelect,
          update: projectUpdate,
        };
      default:
        throw new Error(`Unexpected table ${table}`);
    }
  });

  return {
    client: { from },
    projectUpdate,
  };
}

describe("switchProjectArtifactStorageProvider", () => {
  beforeEach(() => {
    process.env.SUPABASE_API_URL = "https://example.supabase.co";
    mocks.getArtifactStorageConnection.mockReset();
    mocks.syncArtifactThroughRunner.mockReset();
  });

  it("fails with a scope-aware error when the only remote source is outside the current runner scope", async () => {
    const supabase = createSupabaseStub([
      {
        id: "artifact-1",
        project_id: "project-1",
        workflow_run_id: "run-1",
        workflow_run_step_id: "step-1",
        artifact_definition_key: "workflow_output",
        title: "Plan.md",
        created_at: "2026-06-05T00:00:00.000Z",
        updated_at: "2026-06-05T00:00:00.000Z",
      },
    ]);
    mocks.getArtifactStorageConnection.mockResolvedValue({
      projectId: "project-1",
      provider: "google_drive",
      status: "connected",
      folderId: "folder-current",
      folderName: "Current Folder",
      oauthAccountEmail: "runner@example.com",
      lastValidatedAt: "",
      lastError: "",
      connectedAt: "",
      updatedAt: "",
    });

    const result = await switchProjectArtifactStorageProvider("project-1", "supabase", {
      supabase: supabase.client as never,
      localRunnerGateway: {
        listArtifacts: vi.fn().mockResolvedValue([]),
        getArtifactById: vi.fn().mockResolvedValue(null),
        exportArtifactSyncBundle: vi.fn(),
        saveArtifactCloudSyncResult: vi.fn(),
        hydrateArtifactFromRemote: vi.fn(),
      },
      artifactStorageConnectionGateway: {
        getProjectStorageProvider: vi.fn().mockResolvedValue("google_drive"),
        updateArtifactRunStorageMetadata: vi.fn(),
        listArtifactRunReplicas: vi.fn().mockResolvedValue({
          "artifact-1": [
            {
              id: "replica-1",
              artifactRunId: "artifact-1",
              projectId: "project-1",
              provider: "google_drive",
              storageScopeKey: "gdrive:folder-other",
              remotePath: "projects/project-1/runs/run-1/steps/step-1/Plan.md",
              remoteObjectId: "drive-object-1",
              syncStatus: "synced",
              checksum: null,
              lastSyncedAt: "2026-06-05T00:00:00.000Z",
              lastError: null,
              createdAt: "2026-06-05T00:00:00.000Z",
              updatedAt: "2026-06-05T00:00:00.000Z",
            },
          ],
        }),
      },
    });

    expect(result.status).toBe("failed");
    expect(result.failedCount).toBe(1);
    expect(result.failures[0]?.reason).toContain("current runner storage scope");
    expect(mocks.syncArtifactThroughRunner).not.toHaveBeenCalled();
    expect(supabase.projectUpdate).not.toHaveBeenCalled();
  });

  it("hydrates from a matching remote scope before syncing to the target provider", async () => {
    const supabase = createSupabaseStub([
      {
        id: "artifact-1",
        project_id: "project-1",
        workflow_run_id: "run-1",
        workflow_run_step_id: "step-1",
        artifact_definition_key: "workflow_output",
        title: "Plan.md",
        created_at: "2026-06-05T00:00:00.000Z",
        updated_at: "2026-06-05T00:00:00.000Z",
      },
    ]);
    mocks.getArtifactStorageConnection.mockResolvedValue({
      projectId: "project-1",
      provider: "google_drive",
      status: "connected",
      folderId: "folder-current",
      folderName: "Current Folder",
      oauthAccountEmail: "runner@example.com",
      lastValidatedAt: "",
      lastError: "",
      connectedAt: "",
      updatedAt: "",
    });
    mocks.syncArtifactThroughRunner.mockResolvedValue({
      storageProvider: "supabase",
      remotePath: "projects/project-1/runs/run-1/steps/step-1/Plan.md",
      syncStatus: "synced",
    });

    const hydrateArtifactFromRemote = vi.fn().mockResolvedValue({
      artifactId: "artifact-1",
      projectId: "project-1",
      workflowRunId: "run-1",
      workflowStepKey: "step-1",
      title: "Plan.md",
      sourceKind: "workflow_output",
      providerKey: "codex",
      localPath: ".flowpilot/artifacts/project-1/run-1/step-1/artifact-1",
      remotePath: "projects/project-1/runs/run-1/steps/step-1/Plan.md",
      remoteUrl: "",
      syncStatus: "local_only",
      createdAt: "2026-06-05T00:00:00.000Z",
      updatedAt: "2026-06-05T00:00:00.000Z",
      contentMarkdown: "",
      previewMarkdown: "",
    });

    const result = await switchProjectArtifactStorageProvider("project-1", "supabase", {
      supabase: supabase.client as never,
      localRunnerGateway: {
        listArtifacts: vi.fn().mockResolvedValue([]),
        getArtifactById: vi.fn().mockResolvedValue(null),
        exportArtifactSyncBundle: vi.fn(),
        saveArtifactCloudSyncResult: vi.fn(),
        hydrateArtifactFromRemote,
      },
      artifactStorageConnectionGateway: {
        getProjectStorageProvider: vi.fn().mockResolvedValue("google_drive"),
        updateArtifactRunStorageMetadata: vi.fn(),
        listArtifactRunReplicas: vi.fn().mockResolvedValue({
          "artifact-1": [
            {
              id: "replica-1",
              artifactRunId: "artifact-1",
              projectId: "project-1",
              provider: "google_drive",
              storageScopeKey: "gdrive:folder-current",
              remotePath: "projects/project-1/runs/run-1/steps/step-1/Plan.md",
              remoteObjectId: "drive-object-1",
              syncStatus: "synced",
              checksum: null,
              lastSyncedAt: "2026-06-05T00:00:00.000Z",
              lastError: null,
              createdAt: "2026-06-05T00:00:00.000Z",
              updatedAt: "2026-06-05T00:00:00.000Z",
            },
          ],
        }),
      },
    });

    expect(hydrateArtifactFromRemote).toHaveBeenCalledWith(
      "artifact-1",
      expect.objectContaining({
        sourceStorageProvider: "google_drive",
        remoteObjectId: "drive-object-1",
      }),
    );
    expect(mocks.syncArtifactThroughRunner).toHaveBeenCalledWith(
      "artifact-1",
      expect.any(Object),
      { targetProvider: "supabase" },
    );
    expect(result.status).toBe("completed");
    expect(supabase.projectUpdate).toHaveBeenCalled();
  });
});
