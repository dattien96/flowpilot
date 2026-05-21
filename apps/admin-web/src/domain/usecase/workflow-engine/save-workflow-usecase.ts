import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type { Workflow, WorkflowStep } from "@/domain/model/entity/workflow-engine";

export class SaveWorkflowUseCase {
  constructor(private readonly gateway: WorkflowEngineGateway) {}

  async execute(
    workflow: Omit<Partial<Workflow>, "steps"> & { steps: Partial<WorkflowStep>[] }
  ): Promise<Workflow> {
    return this.gateway.saveWorkflow(workflow);
  }
}
