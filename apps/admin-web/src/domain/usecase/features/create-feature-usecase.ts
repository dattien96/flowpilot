import type { FeatureGateway } from "@/domain/gateway/feature-gateway";
import type { CreateFeaturePayload } from "@/domain/model/payload/feature-payload";

export class CreateFeatureUseCase {
  constructor(private readonly featureGateway: FeatureGateway) {}

  execute(payload: CreateFeaturePayload) {
    return this.featureGateway.createFeature(payload);
  }
}
