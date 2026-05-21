import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type { Workflow } from "@/domain/model/entity/workflow-engine";

export class GetWorkflowDetailUseCase {
  constructor(private readonly gateway: WorkflowEngineGateway) {}

  async execute(workflowId: string): Promise<Workflow | null> {
    if (!workflowId) throw new Error("Workflow ID is required.");
    return this.gateway.getWorkflowDetail(workflowId);
  }
}
