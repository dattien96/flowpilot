import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type { StepDefinition } from "@/domain/model/entity/workflow-engine";

export class ListStepDefinitionsUseCase {
  constructor(private readonly gateway: WorkflowEngineGateway) {}

  async execute(): Promise<StepDefinition[]> {
    return this.gateway.listStepDefinitions();
  }
}
