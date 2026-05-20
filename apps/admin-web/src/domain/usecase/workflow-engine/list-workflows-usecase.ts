import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type { Workflow } from "@/domain/model/entity/workflow-engine";

export class ListWorkflowsUseCase {
  constructor(private readonly gateway: WorkflowEngineGateway) {}

  async execute(projectId?: string): Promise<Workflow[]> {
    return this.gateway.listWorkflows(projectId);
  }
}
