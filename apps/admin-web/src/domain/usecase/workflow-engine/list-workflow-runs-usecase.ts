import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type { WorkflowRun } from "@/domain/model/entity/workflow-engine";

export class ListWorkflowRunsUseCase {
  constructor(private readonly gateway: WorkflowEngineGateway) {}

  async execute(projectId?: string): Promise<WorkflowRun[]> {
    return this.gateway.listWorkflowRuns(projectId);
  }
}
