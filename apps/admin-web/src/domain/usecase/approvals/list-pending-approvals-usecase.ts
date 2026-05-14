import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";

export class ListPendingApprovalsUseCase {
  constructor(private readonly workflowGateway: WorkflowGateway) {}

  async execute() {
    const runs = await this.workflowGateway.listWorkflowRuns();
    const details = await Promise.all(
      runs
        .filter((run) => run.status === "waiting_approval")
        .map((run) => this.workflowGateway.getWorkflowRunDetail(run.id)),
    );

    return details
      .flatMap((detail) => detail?.approvals ?? [])
      .filter((approval) => approval.status === "pending");
  }
}
