import type { WorkflowGateway } from "@/domain/gateway/workflow-gateway";
import type { ListOutputsFilters } from "@/domain/model/payload/workflow-payload";

export class ListOutputsUseCase {
  constructor(private readonly workflowGateway: WorkflowGateway) {}

  execute(filters?: ListOutputsFilters) {
    return this.workflowGateway.listOutputs(filters);
  }
}
