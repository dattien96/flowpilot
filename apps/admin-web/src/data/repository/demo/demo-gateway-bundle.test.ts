import { afterEach, describe, expect, it } from "vitest";

import { createDemoGatewayBundle } from "./demo-gateway-bundle";
import {
  demoApprovalDecisions,
  demoApprovals,
  demoIntegrations,
  demoLogs,
  demoOutputs,
  demoWorkflowRuns,
  demoWorkflowSteps,
} from "./demo-store";

const integrationsSnapshot = structuredClone(demoIntegrations);
const workflowRunsSnapshot = structuredClone(demoWorkflowRuns);
const workflowStepsSnapshot = structuredClone(demoWorkflowSteps);
const outputsSnapshot = structuredClone(demoOutputs);
const approvalsSnapshot = structuredClone(demoApprovals);
const approvalDecisionsSnapshot = structuredClone(demoApprovalDecisions);
const logsSnapshot = structuredClone(demoLogs);

afterEach(() => {
  demoIntegrations.splice(0, demoIntegrations.length, ...structuredClone(integrationsSnapshot));
  demoWorkflowRuns.splice(0, demoWorkflowRuns.length, ...structuredClone(workflowRunsSnapshot));
  demoWorkflowSteps.splice(0, demoWorkflowSteps.length, ...structuredClone(workflowStepsSnapshot));
  demoOutputs.splice(0, demoOutputs.length, ...structuredClone(outputsSnapshot));
  demoApprovals.splice(0, demoApprovals.length, ...structuredClone(approvalsSnapshot));
  demoApprovalDecisions.splice(0, demoApprovalDecisions.length, ...structuredClone(approvalDecisionsSnapshot));
  demoLogs.splice(0, demoLogs.length, ...structuredClone(logsSnapshot));
});

describe("DemoGatewayBundle integrations", () => {
  it("lists only integrations belonging to the requested project", async () => {
    const gateway = createDemoGatewayBundle().integrationGateway;

    const integrations = await gateway.listIntegrationsByProject("project_meal_suggestion");

    expect(integrations).toHaveLength(2);
    expect(integrations.every((integration) => integration.projectId === "project_meal_suggestion")).toBe(true);
  });

  it("creates a pending integration with label and config", async () => {
    const gateway = createDemoGatewayBundle().integrationGateway;

    const created = await gateway.createIntegration({
      projectId: "project_meal_suggestion",
      type: "figma",
      label: "Design System",
      configEncrypted: { fileKey: "abc123" },
    });

    expect(created.status).toBe("pending");
    expect(created.lastError).toBeNull();
    expect(demoIntegrations.some((integration) => integration.id === created.id)).toBe(true);
  });

  it("updates integration label, status, and error fields", async () => {
    const gateway = createDemoGatewayBundle().integrationGateway;
    const original = demoIntegrations[0];

    const updated = await gateway.updateIntegration(original.id, {
      label: "Updated Label",
      status: "failed",
      lastError: "runner offline",
    });

    expect(updated.label).toBe("Updated Label");
    expect(updated.status).toBe("failed");
    expect(updated.lastError).toBe("runner offline");
    expect(updated.updatedAt).not.toBe(original.createdAt);
  });

  it("clears lastError when a retry resets the integration back to pending", async () => {
    const gateway = createDemoGatewayBundle().integrationGateway;
    const target = demoIntegrations.find((integration) => integration.status === "failed");
    if (!target) {
      throw new Error("Expected a failed seeded integration.");
    }

    const updated = await gateway.updateIntegration(target.id, {
      status: "pending",
    });

    expect(updated.status).toBe("pending");
    expect(updated.lastError).toBeNull();
  });

  it("deletes an integration by id", async () => {
    const gateway = createDemoGatewayBundle().integrationGateway;
    const targetId = demoIntegrations[0].id;

    await gateway.deleteIntegration(targetId);

    expect(demoIntegrations.some((integration) => integration.id === targetId)).toBe(false);
  });
});

describe("DemoGatewayBundle workflow runs", () => {
  it("deletes workflow runs and cascades related in-memory records", async () => {
    const gateway = createDemoGatewayBundle().workflowGateway;

    demoWorkflowRuns.push({
      id: "run-1",
      workflowDefinitionId: "workflow-1",
      projectId: "project-1",
      status: "completed",
      currentStepKey: null,
      selectedContextSourceIds: [],
      startedBy: "demo-user",
      startedAt: "2026-05-23T00:00:00.000Z",
      completedAt: "2026-05-23T00:01:00.000Z",
      errorSummary: null,
    });
    demoWorkflowSteps.push({
      id: "step-1",
      workflowRunId: "run-1",
      stepKey: "business_idea",
      stepName: "Business Idea",
      stepType: "ai_mock",
      status: "completed",
      sequenceIndex: 0,
      outputId: null,
      startedAt: null,
      completedAt: null,
      errorMessage: null,
    });
    demoOutputs.push({
      id: "output-1",
      workflowRunId: "run-1",
      workflowStepId: "step-1",
      projectId: "project-1",
      outputType: "document",
      version: 1,
      title: "BusinessIdea.md",
      contentMarkdown: "# Business Idea",
      isApproved: false,
      createdAt: "2026-05-23T00:01:00.000Z",
    });
    demoApprovals.push({
      id: "approval-1",
      workflowRunId: "run-1",
      workflowStepId: "step-1",
      aiOutputId: "output-1",
      status: "pending",
      reviewerId: null,
      comment: null,
      decidedAt: null,
      createdAt: "2026-05-23T00:01:00.000Z",
    });
    demoApprovalDecisions.push({
      id: "decision-1",
      approvalId: "approval-1",
      workflowRunId: "run-1",
      workflowStepId: "step-1",
      aiOutputId: "output-1",
      decision: "approved",
      reviewerId: "demo-user",
      comment: null,
      createdAt: "2026-05-23T00:01:00.000Z",
    });
    demoLogs.push({
      id: "log-1",
      workflowRunId: "run-1",
      workflowStepId: "step-1",
      provider: "codex",
      model: "gpt-5.4",
      status: "success",
      inputTokens: 1,
      outputTokens: 1,
      costEstimate: 0,
      latencyMs: 1,
      createdAt: "2026-05-23T00:01:00.000Z",
    });

    await gateway.deleteWorkflowRuns(["run-1"]);

    expect(demoWorkflowRuns.some((run) => run.id === "run-1")).toBe(false);
    expect(demoWorkflowSteps.some((step) => step.workflowRunId === "run-1")).toBe(false);
    expect(demoOutputs.some((output) => output.workflowRunId === "run-1")).toBe(false);
    expect(demoApprovals.some((approval) => approval.workflowRunId === "run-1")).toBe(false);
    expect(demoApprovalDecisions.some((decision) => decision.workflowRunId === "run-1")).toBe(false);
    expect(demoLogs.some((log) => log.workflowRunId === "run-1")).toBe(false);
  });
});
