import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";

export class GetOutputDetailUseCase {
  constructor(private readonly workflowGateway: WorkflowGateway) {}

  execute(outputId: string) {
    return this.workflowGateway.getOutputDetail(outputId);
  }
}
