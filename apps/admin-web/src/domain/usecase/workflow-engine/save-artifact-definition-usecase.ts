import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type { ArtifactDefinition } from "@/domain/model/entity/workflow-engine";

export class SaveArtifactDefinitionUseCase {
  constructor(private readonly gateway: WorkflowEngineGateway) {}

  async execute(definition: ArtifactDefinition): Promise<ArtifactDefinition> {
    return this.gateway.saveArtifactDefinition(definition);
  }
}
