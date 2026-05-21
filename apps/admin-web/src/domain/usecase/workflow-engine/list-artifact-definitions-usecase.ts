import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type { ArtifactDefinition } from "@/domain/model/entity/workflow-engine";

export class ListArtifactDefinitionsUseCase {
  constructor(private readonly gateway: WorkflowEngineGateway) {}

  async execute(): Promise<ArtifactDefinition[]> {
    return this.gateway.listArtifactDefinitions();
  }
}
