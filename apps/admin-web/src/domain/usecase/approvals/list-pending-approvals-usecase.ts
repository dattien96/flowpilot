import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";

export class ListPendingApprovalsUseCase {
  constructor(private readonly workflowGateway: WorkflowGateway) {}

  async execute() {
    return this.workflowGateway.listPendingApprovalDetails();
  }
}
