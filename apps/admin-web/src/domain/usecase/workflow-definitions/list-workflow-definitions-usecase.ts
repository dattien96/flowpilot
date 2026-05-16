import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";

export class ListWorkflowDefinitionsUseCase {
  constructor(private readonly workflowGateway: WorkflowGateway) {}

  execute() {
    return this.workflowGateway.listWorkflowDefinitions();
  }
}
