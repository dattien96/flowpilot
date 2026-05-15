import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";

export class GetWorkflowRunDetailUseCase {
  constructor(private readonly workflowGateway: WorkflowGateway) {}

  execute(runId: string) {
    return this.workflowGateway.getWorkflowRunDetail(runId);
  }
}
