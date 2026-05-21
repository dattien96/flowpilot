import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type { StepDefinition } from "@/domain/model/entity/workflow-engine";

export class SaveStepDefinitionUseCase {
  constructor(private readonly gateway: WorkflowEngineGateway) {}

  async execute(step: StepDefinition): Promise<StepDefinition> {
    return this.gateway.saveStepDefinition(step);
  }
}
