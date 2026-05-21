import type { WorkflowEngineGateway } from "@/domain/gateway/workflow-engine-gateway";
import type { ArtifactRun } from "@/domain/model/entity/workflow-engine";

export class ListArtifactRunsUseCase {
  constructor(private readonly gateway: WorkflowEngineGateway) {}

  async execute(projectId?: string): Promise<ArtifactRun[]> {
    return this.gateway.listArtifactRuns(projectId);
  }
}
