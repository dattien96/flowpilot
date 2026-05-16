import type { FeatureGateway } from "@/domain/gateway/feature-gateway";
import type { ProjectGateway } from "@/domain/gateway/project-gateway";
import type { CreateFeaturePayload } from "@/domain/model/payload/feature-payload";

export class CreateFeatureUseCase {
  constructor(
    private readonly featureGateway: FeatureGateway,
    private readonly projectGateway: ProjectGateway,
  ) {}

  async execute(payload: CreateFeaturePayload) {
    const project = await this.projectGateway.getProjectById(payload.projectId);

    if (!project) {
      throw new Error("Project not found.");
    }

    return this.featureGateway.createFeature(payload);
  }
}
