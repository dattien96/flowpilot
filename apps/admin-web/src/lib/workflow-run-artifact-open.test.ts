import { describe, expect, it, vi } from "vitest";

import type { ArtifactRun } from "@/domain/model/entity/workflow-engine";

import {
  buildWorkflowRunArtifactOpenHref,
  loadWorkflowRunArtifactPromptFiles,
} from "./workflow-run-artifact-open";

const artifactRun: ArtifactRun = {
  id: "artifact-remote-1",
  artifactDefinitionKey: "workflow_output",
  workflowId: "workflow-1",
  workflowRunId: "run-1",
  workflowRunStepId: "step-1",
  projectId: "project-1",
  title: "Response.md",
  localPath: "/tmp/remote.md",
  remotePath: "projects/project-1/runs/run-1/steps/plan/Response.md",
  remoteUrl: "https://example.com/remote.md",
  storageProvider: "supabase",
  remoteObjectId: "object-1",
  syncStatus: "synced",
  replicas: [],
  createdAt: "2026-06-02T00:00:00.000Z",
  updatedAt: "2026-06-02T00:00:00.000Z",
};

describe("workflow-run-artifact-open", () => {
  it("builds content URLs with remote metadata by default", () => {
    expect(buildWorkflowRunArtifactOpenHref(artifactRun)).toBe(
      "/api/local-runner/artifacts/artifact-remote-1/open?remotePath=projects%2Fproject-1%2Fruns%2Frun-1%2Fsteps%2Fplan%2FResponse.md&storageProvider=supabase&remoteObjectId=object-1&projectId=project-1",
    );
  });

  it("builds prompt file URLs with remote metadata", () => {
    expect(
      buildWorkflowRunArtifactOpenHref(artifactRun, "actual-prompt"),
    ).toBe(
      "/api/local-runner/artifacts/artifact-remote-1/open?remotePath=projects%2Fproject-1%2Fruns%2Frun-1%2Fsteps%2Fplan%2FResponse.md&storageProvider=supabase&remoteObjectId=object-1&projectId=project-1&file=actual-prompt",
    );
  });

  it("loads both prompt files independently and tolerates missing actual prompt", async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(new Response("Not found", { status: 404 }))
      .mockResolvedValueOnce(new Response("Saved prompt", { status: 200 }));

    await expect(
      loadWorkflowRunArtifactPromptFiles(artifactRun, fetchMock),
    ).resolves.toEqual({
      actualPromptText: null,
      promptText: "Saved prompt",
    });

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/api/local-runner/artifacts/artifact-remote-1/open?remotePath=projects%2Fproject-1%2Fruns%2Frun-1%2Fsteps%2Fplan%2FResponse.md&storageProvider=supabase&remoteObjectId=object-1&projectId=project-1&file=actual-prompt",
      {
        cache: "no-store",
        headers: {
          accept: "text/markdown",
        },
      },
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/api/local-runner/artifacts/artifact-remote-1/open?remotePath=projects%2Fproject-1%2Fruns%2Frun-1%2Fsteps%2Fplan%2FResponse.md&storageProvider=supabase&remoteObjectId=object-1&projectId=project-1&file=prompt",
      {
        cache: "no-store",
        headers: {
          accept: "text/markdown",
        },
      },
    );
  });
});
