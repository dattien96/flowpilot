import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type { WorkflowRun } from "@/domain/model/entity/workflow-engine";

export class StartWorkflowRunUseCase {
  constructor(private readonly gateway: WorkflowEngineGateway) {}

  async execute(workflowId: string, projectId: string): Promise<WorkflowRun> {
    if (!workflowId || !projectId) throw new Error("Workflow ID and Project ID are required.");
    return this.gateway.startWorkflowRun(workflowId, projectId);
  }
}
