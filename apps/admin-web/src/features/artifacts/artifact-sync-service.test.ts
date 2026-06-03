import { beforeEach, describe, expect, it, vi } from "vitest";

import { syncArtifactThroughRunner } from "./artifact-sync-service";

const mocks = vi.hoisted(() => ({
  syncSnapshot: vi.fn(),
  createOpenUrl: vi.fn(),
  getArtifactStorageConnection: vi.fn(),
}));

vi.mock("@/data/repository/supabase/supabase-artifact-storage-gateway", () => ({
  SupabaseArtifactStorageGateway: vi.fn().mockImplementation(() => ({
    syncSnapshot: mocks.syncSnapshot,
    createOpenUrl: mocks.createOpenUrl,
  })),
}));

vi.mock("@/features/artifacts/artifact-storage-connection", () => ({
  getArtifactStorageConnection: mocks.getArtifactStorageConnection,
}));

describe("syncArtifactThroughRunner", () => {
  beforeEach(() => {
    mocks.syncSnapshot.mockReset();
    mocks.createOpenUrl.mockReset();
    mocks.getArtifactStorageConnection.mockReset();
  });

  it("fails before uploading when the artifact has no shared sync row", async () => {
    const getArtifactById = vi.fn().mockResolvedValue({
      artifactId: "artifact-no-shared-row",
      title: "Plan",
      sourceKind: "workflow_output",
      projectId: "project-alpha",
      workflowRunId: "run-1",
      workflowStepKey: "plan",
      providerKey: "codex",
      localPath: ".flowpilot/artifacts/project-alpha/run-1/plan",
      remotePath: "",
      remoteUrl: "",
      syncStatus: "local_only",
      createdAt: "2026-06-01T00:00:00.000Z",
      updatedAt: "2026-06-01T00:00:00.000Z",
      contentMarkdown: "",
      previewMarkdown: "",
      titlePath: "",
    });
    const exportArtifactSyncBundle = vi.fn();
    const saveArtifactCloudSyncResult = vi.fn();

    const supabase = {
      from(table: string) {
        if (table !== "projects") {
          throw new Error(`Unexpected table ${table}`);
        }
        return {
          select: () => ({
            eq: () => ({
              maybeSingle: async () => ({
                data: { artifact_storage_preference: "supabase" },
                error: null,
              }),
            }),
          }),
        };
      },
    } as never;

    const updateArtifactRunStorageMetadata = vi
      .fn()
      .mockRejectedValue(
        new Error(
          'Artifact run "artifact-no-shared-row" was not found in shared sync state. Only workflow-backed artifacts can appear in Remote/Synced across PCs.',
        ),
      );

    await expect(
      syncArtifactThroughRunner("artifact-no-shared-row", {
        supabase,
        localRunnerGateway: {
          getArtifactById,
          listArtifacts: vi.fn(),
          exportArtifactSyncBundle,
          saveArtifactCloudSyncResult,
        },
        artifactStorageConnectionGateway: {
          updateArtifactRunStorageMetadata,
        },
      }),
    ).rejects.toThrow(/workflow-backed artifacts/i);

    expect(exportArtifactSyncBundle).not.toHaveBeenCalled();
    expect(mocks.syncSnapshot).not.toHaveBeenCalled();
    expect(saveArtifactCloudSyncResult).not.toHaveBeenCalled();
  });
});
