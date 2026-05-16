import { describe, expect, it, vi } from "vitest";

import type { WorkflowExecutorGateway, WorkflowGateway } from "@/domain/gateway/workflow-gateway";
import type {
  Approval,
  ApprovalDecision,
  WorkflowRun,
  WorkflowStep,
} from "@/domain/model/entity/workflow";
import { SubmitApprovalDecisionUseCase } from "./submit-approval-decision-usecase";

const approval: Approval = {
  id: "approval_1",
  workflowRunId: "run_1",
  workflowStepId: "step_approval",
  aiOutputId: "output_1",
  status: "pending",
  reviewerId: null,
  comment: null,
  decidedAt: null,
  createdAt: "2026-05-15T00:00:00.000Z",
};

const run: WorkflowRun = {
  id: "run_1",
  workflowDefinitionId: "workflow_1",
  projectId: "project_1",
  featureId: "feature_1",
  status: "waiting_approval",
  currentStepKey: "approval_business_summary",
  selectedContextSourceIds: [],
  startedBy: "test",
  startedAt: "2026-05-15T00:00:00.000Z",
  completedAt: null,
  errorSummary: null,
};

const approvalStep: WorkflowStep = {
  id: "step_approval",
  workflowRunId: "run_1",
  stepKey: "approval_business_summary",
  stepName: "Approve Business Summary",
  stepType: "approval",
  status: "waiting_approval",
  sequenceIndex: 2,
  outputId: "output_1",
  startedAt: "2026-05-15T00:00:00.000Z",
  completedAt: null,
  errorMessage: null,
};

function createWorkflowGateway() {
  return {
    getApprovalById: vi.fn(async () => approval),
    getWorkflowRunById: vi.fn(async () => run),
    getWorkflowRunDetail: vi.fn(async () => ({
      run,
      steps: [approvalStep],
      approvals: [approval],
      outputs: [],
      logs: [],
      approvalDecisions: [],
    })),
    updateApproval: vi.fn(async () => ({ ...approval, status: "changes_requested" })),
    updateWorkflowStep: vi.fn(async () => approvalStep),
    updateWorkflowRun: vi.fn(async (_runId: string, patch: Partial<WorkflowRun>) => ({
      ...run,
      ...patch,
    })),
    createApprovalDecision: vi.fn(async (decision: ApprovalDecision) => decision),
  } as unknown as WorkflowGateway;
}

describe("SubmitApprovalDecisionUseCase", () => {
  it("records decision history and keeps currentStepKey as a step key", async () => {
    const workflowGateway = createWorkflowGateway();
    const executor = {
      executeUntilPause: vi.fn(),
    } as WorkflowExecutorGateway;

    await new SubmitApprovalDecisionUseCase(workflowGateway, executor).execute({
      approvalId: "approval_1",
      decision: "changes_requested",
      comment: "Needs revision",
    });

    expect(workflowGateway.createApprovalDecision).toHaveBeenCalledWith(
      expect.objectContaining({
        approvalId: "approval_1",
        decision: "changes_requested",
        comment: "Needs revision",
      }),
    );
    expect(workflowGateway.updateWorkflowRun).toHaveBeenCalledWith(
      "run_1",
      expect.objectContaining({
        currentStepKey: "approval_business_summary",
      }),
    );
  });
});
