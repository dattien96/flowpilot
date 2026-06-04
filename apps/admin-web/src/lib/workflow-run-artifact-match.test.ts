import { describe, expect, it } from "vitest";

import type { ArtifactRun } from "@/domain/model/entity/workflow-engine";
import type { WorkflowOutputRecord } from "@/features/workflow-engine/workflow-run-detail-timeline";

import { resolveArtifactRunForOutput } from "./workflow-run-artifact-match";

const remoteArtifactRuns: ArtifactRun[] = [
  {
    id: "artifact-1",
    artifactDefinitionKey: "workflow_output",
    workflowId: "workflow-1",
    workflowRunId: "run-1",
    workflowRunStepId: "step-1",
    projectId: "project-1",
    title: "Response.md",
    localPath: "",
    remotePath: "projects/project-1/runs/run-1/steps/step-1/Response.md",
    remoteUrl: "",
    storageProvider: "supabase",
    remoteObjectId: "object-1",
    syncStatus: "synced",
    createdAt: "2026-06-04T00:00:02.000Z",
    updatedAt: "2026-06-04T00:00:02.000Z",
  },
  {
    id: "artifact-2",
    artifactDefinitionKey: "workflow_output",
    workflowId: "workflow-1",
    workflowRunId: "run-1",
    workflowRunStepId: "step-2",
    projectId: "project-1",
    title: "Plan.md",
    localPath: "",
    remotePath: "projects/project-1/runs/run-1/steps/step-2/Plan.md",
    remoteUrl: "",
    storageProvider: "supabase",
    remoteObjectId: "object-2",
    syncStatus: "synced",
    createdAt: "2026-06-04T00:10:02.000Z",
    updatedAt: "2026-06-04T00:10:02.000Z",
  },
];

describe("resolveArtifactRunForOutput", () => {
  it("prefers exact id matches", () => {
    const output: WorkflowOutputRecord = {
      id: "artifact-2",
      workflowRunId: "run-1",
      workflowStepId: "step-2",
      projectId: "project-1",
      outputType: "document",
      version: 1,
      title: "Plan.md",
      contentMarkdown: "ok",
      isApproved: true,
      createdAt: "2026-06-04T00:10:01.000Z",
    };

    expect(resolveArtifactRunForOutput(output, remoteArtifactRuns)?.id).toBe("artifact-2");
  });

  it("falls back to step metadata when the output id is synthesized", () => {
    const output: WorkflowOutputRecord = {
      id: "log-output-step-1",
      workflowRunId: "run-1",
      workflowStepId: "step-1",
      projectId: "project-1",
      outputType: "document",
      version: 1,
      title: "Response.md",
      contentMarkdown: "Recovered content",
      isApproved: true,
      createdAt: "2026-06-04T00:00:00.000Z",
    };

    expect(resolveArtifactRunForOutput(output, remoteArtifactRuns)?.id).toBe("artifact-1");
  });
});
