import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";

export class GetWorkflowDefinitionDetailUseCase {
  constructor(private readonly workflowGateway: WorkflowGateway) {}

  execute(workflowDefinitionId: string) {
    return this.workflowGateway.getWorkflowDefinitionById(workflowDefinitionId);
  }
}
