import type { FeatureGateway } from "@/domain/gateway/feature-gateway";

export class ListFeaturesUseCase {
  constructor(private readonly featureGateway: FeatureGateway) {}

  execute() {
    return this.featureGateway.listFeatures();
  }
}
