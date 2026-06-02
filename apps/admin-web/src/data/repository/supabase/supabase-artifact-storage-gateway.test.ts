import { describe, expect, it, vi } from "vitest";

import { SupabaseArtifactStorageGateway } from "./supabase-artifact-storage-gateway";

function createSupabaseStub(
  infoResponses: Array<{ data: Record<string, unknown> | null; error: { message: string } | null }> = [],
) {
  const upload = vi.fn(async (path: string) => ({
    data: {
      id: path.includes("/.snapshots/") ? `${path}:snapshot-id` : "canonical-upload-id",
    },
    error: null,
  }));
  const info = vi.fn(async () => infoResponses.shift() ?? { data: null, error: null });
  const createSignedUrl = vi.fn(async () => ({
    data: { signedUrl: "https://example.test/signed" },
    error: null,
  }));
  const bucket = { upload, info, createSignedUrl };
  const from = vi.fn(() => bucket);

  return {
    client: {
      storage: {
        from,
      },
    },
    upload,
    info,
    createSignedUrl,
    from,
  };
}

describe("SupabaseArtifactStorageGateway", () => {
  it("uploads snapshot files and promotes the canonical output when the snapshot is newest", async () => {
    const stub = createSupabaseStub([
      { data: null, error: null },
      {
        data: {
          id: "existing-canonical",
          metadata: { sourceCreatedAt: "2026-06-02T08:59:59.000Z" },
        },
        error: null,
      },
    ]);
    const gateway = new SupabaseArtifactStorageGateway(stub.client as never);

    const result = await gateway.syncSnapshot({
      projectId: "project-alpha",
      artifactId: "artifact-1",
      workflowRunId: "run-1",
      workflowStepKey: "business_idea",
      outputFilename: "content.md",
      createdAt: "2026-06-02T09:00:00.000Z",
      files: [
        { relativePath: "content.md", bytes: new TextEncoder().encode("hello") },
        { relativePath: "manifest.json", bytes: new TextEncoder().encode("{}") },
      ],
    });

    expect(stub.from).toHaveBeenCalledWith("flowpilot-artifacts");
    expect(stub.upload.mock.calls.map((call) => call[0])).toEqual([
      "projects/project-alpha/runs/run-1/steps/business_idea/.snapshots/artifact-1/content.md",
      "projects/project-alpha/runs/run-1/steps/business_idea/.snapshots/artifact-1/manifest.json",
      "projects/project-alpha/runs/run-1/steps/business_idea/content.md",
    ]);
    expect(result).toEqual({
      storageProvider: "supabase",
      remotePath: "projects/project-alpha/runs/run-1/steps/business_idea/content.md",
      remoteObjectId: "canonical-upload-id",
    });
  });

  it("keeps the canonical file stable when an older snapshot retries", async () => {
    const stub = createSupabaseStub([
      {
        data: {
          id: "existing-canonical",
          metadata: { sourceCreatedAt: "2026-06-02T09:10:00.000Z" },
        },
        error: null,
      },
      {
        data: {
          id: "existing-canonical",
          metadata: { sourceCreatedAt: "2026-06-02T09:10:00.000Z" },
        },
        error: null,
      },
    ]);
    const gateway = new SupabaseArtifactStorageGateway(stub.client as never);

    const result = await gateway.syncSnapshot({
      projectId: "project-alpha",
      artifactId: "artifact-1",
      workflowRunId: "run-1",
      workflowStepKey: "business_idea",
      outputFilename: "content.md",
      createdAt: "2026-06-02T09:00:00.000Z",
      files: [{ relativePath: "content.md", bytes: new TextEncoder().encode("hello") }],
    });

    expect(stub.upload.mock.calls.map((call) => call[0])).toEqual([
      "projects/project-alpha/runs/run-1/steps/business_idea/.snapshots/artifact-1/content.md",
    ]);
    expect(result.remotePath).toBe("projects/project-alpha/runs/run-1/steps/business_idea/content.md");
    expect(result.remoteObjectId).toBe("existing-canonical");
  });

  it("creates signed open URLs on demand", async () => {
    const stub = createSupabaseStub();
    const gateway = new SupabaseArtifactStorageGateway(stub.client as never);

    const url = await gateway.createOpenUrl(
      "projects/project-alpha/runs/run-1/steps/business_idea/content.md",
    );

    expect(url).toBe("https://example.test/signed");
    expect(stub.createSignedUrl).toHaveBeenCalledWith(
      "projects/project-alpha/runs/run-1/steps/business_idea/content.md",
      300,
    );
  });
});
