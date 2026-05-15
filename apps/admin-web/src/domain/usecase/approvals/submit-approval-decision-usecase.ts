import type {
  WorkflowExecutorGateway,
  WorkflowGateway,
} from "@/domain/gateway/workflow-gateway";
import type { SubmitApprovalDecisionPayload } from "@/domain/model/payload/workflow-payload";

export class SubmitApprovalDecisionUseCase {
  constructor(
    private readonly workflowGateway: WorkflowGateway,
    private readonly workflowExecutor: WorkflowExecutorGateway,
  ) {}

  async execute(payload: SubmitApprovalDecisionPayload) {
    const runs = await this.workflowGateway.listWorkflowRuns();

    for (const run of runs) {
      const detail = await this.workflowGateway.getWorkflowRunDetail(run.id);
      const approval = detail?.approvals.find((item) => item.id === payload.approvalId);

      if (!approval) {
        continue;
      }

      if (approval.status !== "pending") {
        throw new Error("Approval already decided.");
      }

      await this.workflowGateway.updateApproval(payload.approvalId, {
        status: payload.decision,
        comment: payload.comment ?? null,
        decidedAt: new Date().toISOString(),
      });

      if (payload.decision === "approved") {
        await this.workflowGateway.updateWorkflowStep(approval.workflowStepId, {
          status: "completed",
          completedAt: new Date().toISOString(),
        });

        await this.workflowGateway.updateWorkflowRun(run.id, {
          status: "running",
          currentStepKey: null,
        });

        await this.workflowExecutor.executeUntilPause(run.id);
      }

      if (payload.decision === "rejected") {
        await this.workflowGateway.updateWorkflowStep(approval.workflowStepId, {
          status: "rejected",
          completedAt: new Date().toISOString(),
        });

        await this.workflowGateway.updateWorkflowRun(run.id, {
          status: "rejected",
          currentStepKey: approval.workflowStepId,
          completedAt: new Date().toISOString(),
        });
      }

      return this.workflowGateway.getWorkflowRunDetail(run.id);
    }

    throw new Error("Approval not found.");
  }
}
