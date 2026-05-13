import type { ContextSource } from "@/domain/model/entity/context-source";
import type { CreateContextSourcePayload } from "@/domain/model/payload/context-source-payload";

export interface ContextSourceGateway {
  listContextSourcesByFeature(featureId: string): Promise<ContextSource[]>;
  listContextSourcesByProject(projectId: string): Promise<ContextSource[]>;
  createContextSource(payload: CreateContextSourcePayload): Promise<ContextSource>;
}
