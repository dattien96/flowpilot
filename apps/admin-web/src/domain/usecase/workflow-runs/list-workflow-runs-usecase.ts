import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";

export class ListWorkflowRunsUseCase {
  constructor(private readonly workflowGateway: WorkflowGateway) {}

  execute() {
    return this.workflowGateway.listWorkflowRuns();
  }
}
