import type {
  WorkflowExecutorGateway,
  WorkflowGateway,
} from "@/domain/gateway/workflow-gateway";
import type { SubmitApprovalDecisionPayload } from "@/domain/model/payload/workflow-payload";

function createDecisionId() {
  return `decision_${crypto.randomUUID().replaceAll("-", "").slice(0, 18)}`;
}

export class SubmitApprovalDecisionUseCase {
  constructor(
    private readonly workflowGateway: WorkflowGateway,
    private readonly workflowExecutor: WorkflowExecutorGateway,
  ) {}

  async execute(payload: SubmitApprovalDecisionPayload) {
    const approval = await this.workflowGateway.getApprovalById(payload.approvalId);

    if (!approval) {
      throw new Error("Approval not found.");
    }

    if (approval.status !== "pending") {
      throw new Error("Approval already decided.");
    }

    const run = await this.workflowGateway.getWorkflowRunById(approval.workflowRunId);

    if (!run) {
      throw new Error("Workflow run not found.");
    }

    const detail = await this.workflowGateway.getWorkflowRunDetail(run.id);
    const approvalStep = detail?.steps.find((step) => step.id === approval.workflowStepId);
    const currentStepKey = approvalStep?.stepKey ?? null;
    const now = new Date().toISOString();

    await this.workflowGateway.updateApproval(payload.approvalId, {
      status: payload.decision,
      comment: payload.comment ?? null,
      decidedAt: now,
    });

    await this.workflowGateway.createApprovalDecision({
      id: createDecisionId(),
      approvalId: approval.id,
      workflowRunId: approval.workflowRunId,
      workflowStepId: approval.workflowStepId,
      aiOutputId: approval.aiOutputId,
      decision: payload.decision,
      reviewerId: null,
      comment: payload.comment ?? null,
      createdAt: now,
    });

    if (payload.decision === "approved") {
      await this.workflowGateway.updateWorkflowStep(approval.workflowStepId, {
        status: "completed",
        completedAt: now,
      });

      await this.workflowGateway.updateWorkflowRun(run.id, {
        status: "running",
        currentStepKey: null,
      });

      await this.workflowExecutor.executeUntilPause(run.id);
    }

    if (payload.decision === "changes_requested") {
      await this.workflowGateway.updateWorkflowStep(approval.workflowStepId, {
        status: "waiting_approval",
        errorMessage: payload.comment ?? "Changes requested.",
      });

      await this.workflowGateway.updateWorkflowRun(run.id, {
        status: "waiting_approval",
        currentStepKey,
      });
    }

    if (payload.decision === "rejected") {
      await this.workflowGateway.updateWorkflowStep(approval.workflowStepId, {
        status: "rejected",
        completedAt: now,
      });

      await this.workflowGateway.updateWorkflowRun(run.id, {
        status: "rejected",
        currentStepKey,
        completedAt: now,
      });
    }

    return this.workflowGateway.getWorkflowRunDetail(run.id);
  }
}
