import type { Feature } from "@/domain/model/entity/feature";
import type { CreateFeaturePayload } from "@/domain/model/payload/feature-payload";

export interface FeatureGateway {
  listFeatures(): Promise<Feature[]>;
  listFeaturesByProject(projectId: string): Promise<Feature[]>;
  getFeatureById(featureId: string): Promise<Feature | null>;
  createFeature(payload: CreateFeaturePayload): Promise<Feature>;
}
