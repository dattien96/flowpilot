import { describe, expect, it } from "vitest";

import { InMemoryWorkflowEngineGateway } from "./in-memory-workflow-engine-gateway";

describe("InMemoryWorkflowEngineGateway", () => {
  it("deletes workflow runs and related in-memory records", async () => {
    const gateway = new InMemoryWorkflowEngineGateway();
    const anyGateway = gateway as any;

    anyGateway.workflowRuns.push({
      id: "run-1",
      workflowId: "workflow-1",
      projectId: "project-1",
      status: "DONE",
      provider: "codex",
      model: "gpt-5.4",
      reasoningEffort: "medium",
      yoloMode: false,
      startedBy: "demo-user",
      startedAt: "2026-05-23T00:00:00.000Z",
      finishedAt: "2026-05-23T00:01:00.000Z",
      errorMessage: null,
    });
    anyGateway.workflowRunSteps.push({
      id: "wrs-1",
      workflowRunId: "run-1",
      workflowStepId: "step-1",
      executionOrderIndex: 0,
      stepType: "business_idea",
      status: "DONE",
      artifactId: "art-1",
      promptCacheId: null,
      rejectionNote: null,
      retryCount: 0,
      startedAt: "2026-05-23T00:00:00.000Z",
      finishedAt: "2026-05-23T00:01:00.000Z",
      errorMessage: null,
    });
    anyGateway.workflowRunLogs.push({
      id: "log-1",
      workflowRunStepId: "wrs-1",
      logLevel: "info",
      message: "Begin prompt: test",
      createdAt: "2026-05-23T00:00:30.000Z",
    });
    anyGateway.artifactRuns.push({
      id: "art-1",
      artifactDefinitionKey: "business_idea_artifact",
      workflowId: "workflow-1",
      workflowRunId: "run-1",
      workflowRunStepId: "wrs-1",
      projectId: "project-1",
      title: "BusinessIdea.md",
      localPath: ".flowpilot/artifacts/project-1/run-1/business_idea.md",
      remotePath: "",
      remoteUrl: "",
      syncStatus: "local_only",
      createdAt: "2026-05-23T00:00:30.000Z",
      updatedAt: "2026-05-23T00:00:30.000Z",
    });

    await gateway.deleteWorkflowRuns(["run-1"]);

    expect(anyGateway.workflowRuns).toHaveLength(0);
    expect(anyGateway.workflowRunSteps).toHaveLength(0);
    expect(anyGateway.workflowRunLogs).toHaveLength(0);
    expect(anyGateway.artifactRuns).toHaveLength(0);
  });
});
