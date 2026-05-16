import type { ContextSourceGateway } from "@/domain/gateway/context-source-gateway";
import type { FeatureGateway } from "@/domain/gateway/feature-gateway";
import type { ProjectGateway } from "@/domain/gateway/project-gateway";
import type { CreateContextSourcePayload } from "@/domain/model/payload/context-source-payload";

export class CreateContextSourceUseCase {
  constructor(
    private readonly contextGateway: ContextSourceGateway,
    private readonly projectGateway: ProjectGateway,
    private readonly featureGateway: FeatureGateway,
  ) {}

  async execute(payload: CreateContextSourcePayload) {
    const project = await this.projectGateway.getProjectById(payload.projectId);

    if (!project) {
      throw new Error("Project not found.");
    }

    if (payload.featureId) {
      const feature = await this.featureGateway.getFeatureById(payload.featureId);

      if (!feature || feature.projectId !== payload.projectId) {
        throw new Error("Feature not found for selected project.");
      }
    }

    return this.contextGateway.createContextSource(payload);
  }
}
