import { describe, expect, it, vi, beforeEach } from "vitest";

import { POST } from "./route";

const mocks = vi.hoisted(() => ({
  createGatewayBundle: vi.fn(),
  createClient: vi.fn(),
  syncArtifactThroughRunner: vi.fn(),
  updateArtifactRunStorageMetadata: vi.fn(),
}));

vi.mock("@/data/repository/factory", () => ({
  createGatewayBundle: mocks.createGatewayBundle,
}));

vi.mock("@supabase/supabase-js", () => ({
  createClient: mocks.createClient,
}));

vi.mock("@/features/artifacts/artifact-sync-service", () => ({
  syncArtifactThroughRunner: mocks.syncArtifactThroughRunner,
}));

vi.mock("@/data/repository/supabase/supabase-artifact-storage-connection-gateway", () => ({
  SupabaseArtifactStorageConnectionGateway: vi.fn().mockImplementation(() => ({
    updateArtifactRunStorageMetadata: mocks.updateArtifactRunStorageMetadata,
  })),
}));

describe("artifact sync route", () => {
  beforeEach(() => {
    process.env.SUPABASE_API_URL = "https://example.supabase.co";
    process.env.SUPABASE_SERVICE_ROLE_KEY = "service-role-key";
    mocks.createClient.mockReturnValue({});
    mocks.updateArtifactRunStorageMetadata.mockResolvedValue(undefined);
    mocks.createGatewayBundle.mockResolvedValue({
      localRunnerGateway: {
        getArtifactById: vi.fn().mockResolvedValue({
          artifactId: "artifact-1",
          projectId: "project-alpha",
          workflowRunId: "run-1",
          title: "Plan",
          localPath: ".flowpilot/artifacts/project-alpha/run-1/Plan.md",
          remotePath: "",
          remoteUrl: "",
          storageProvider: null,
          remoteObjectId: null,
          syncStatus: "local_only",
          createdAt: "2026-06-01T00:00:00.000Z",
          updatedAt: "2026-06-01T00:00:00.000Z",
        }),
        listArtifacts: vi.fn(),
        exportArtifactSyncBundle: vi.fn(),
        saveArtifactCloudSyncResult: vi.fn(),
      },
    });
  });

  it("returns the refreshed runner artifact after a successful sync", async () => {
    mocks.syncArtifactThroughRunner.mockResolvedValue({
      storageProvider: "supabase",
      remotePath: "projects/project-alpha/runs/run-1/steps/plan/Plan.md",
      remoteObjectId: "object-1",
      syncStatus: "synced",
    });
    const updatedArtifact = {
      artifactId: "artifact-1",
      projectId: "project-alpha",
      workflowRunId: "run-1",
      title: "Plan",
      localPath: ".flowpilot/artifacts/project-alpha/run-1/Plan.md",
      remotePath: "projects/project-alpha/runs/run-1/steps/plan/Plan.md",
      remoteUrl: "https://example.test/object-1",
      storageProvider: "supabase",
      remoteObjectId: "object-1",
      syncStatus: "synced",
      createdAt: "2026-06-01T00:00:00.000Z",
      updatedAt: "2026-06-01T00:05:00.000Z",
    };
    mocks.createGatewayBundle.mockResolvedValue({
      localRunnerGateway: {
        getArtifactById: vi.fn().mockResolvedValue(updatedArtifact),
        listArtifacts: vi.fn(),
        exportArtifactSyncBundle: vi.fn(),
        saveArtifactCloudSyncResult: vi.fn(),
      },
    });

    const response = await POST(new Request("http://localhost"), {
      params: Promise.resolve({ artifactId: "artifact-1" }),
    });

    expect(response.status).toBe(200);
    expect(await response.json()).toEqual(updatedArtifact);
  });

  it("persists a failed sync state when the runner reports failure", async () => {
    mocks.syncArtifactThroughRunner.mockResolvedValue({
      storageProvider: "supabase",
      remotePath: "projects/project-alpha/runs/run-1/steps/plan/Plan.md",
      remoteObjectId: "object-1",
      syncStatus: "failed",
    });
    mocks.createGatewayBundle.mockResolvedValue({
      localRunnerGateway: {
        getArtifactById: vi
          .fn()
          .mockResolvedValueOnce({
            artifactId: "artifact-1",
            projectId: "project-alpha",
            workflowRunId: "run-1",
            title: "Plan",
            localPath: ".flowpilot/artifacts/project-alpha/run-1/Plan.md",
            remotePath: "",
            remoteUrl: "",
            storageProvider: null,
            remoteObjectId: null,
            syncStatus: "local_only",
            createdAt: "2026-06-01T00:00:00.000Z",
            updatedAt: "2026-06-01T00:00:00.000Z",
          })
          .mockResolvedValueOnce({
            artifactId: "artifact-1",
            projectId: "project-alpha",
            workflowRunId: "run-1",
            title: "Plan",
            localPath: ".flowpilot/artifacts/project-alpha/run-1/Plan.md",
            remotePath: "projects/project-alpha/runs/run-1/steps/plan/Plan.md",
            remoteUrl: "https://example.test/object-1",
            storageProvider: "supabase",
            remoteObjectId: "object-1",
            syncStatus: "failed",
            createdAt: "2026-06-01T00:00:00.000Z",
            updatedAt: "2026-06-01T00:05:00.000Z",
          }),
        listArtifacts: vi.fn(),
        exportArtifactSyncBundle: vi.fn(),
        saveArtifactCloudSyncResult: vi.fn(),
      },
    });

    const response = await POST(new Request("http://localhost"), {
      params: Promise.resolve({ artifactId: "artifact-1" }),
    });

    expect(response.status).toBe(200);
    expect(mocks.updateArtifactRunStorageMetadata).toHaveBeenCalledWith(
      expect.objectContaining({
        artifactRunId: "artifact-1",
        syncStatus: "failed",
        storageProvider: "supabase",
      }),
    );
    expect(await response.json()).toMatchObject({
      syncStatus: "failed",
      remoteObjectId: "object-1",
    });
  });
});
