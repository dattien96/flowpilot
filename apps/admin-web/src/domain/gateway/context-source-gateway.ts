import type { ContextSource } from "@/domain/model/entity/context-source";
import type {
  CreateContextSourcePayload,
  UpdateContextSourcePayload,
} from "@/domain/model/payload/context-source-payload";

export interface ContextSourceGateway {
  listContextSources(): Promise<ContextSource[]>;
  listContextSourcesByFeature(featureId: string): Promise<ContextSource[]>;
  listContextSourcesByProject(projectId: string): Promise<ContextSource[]>;
  getContextSourceById(contextSourceId: string): Promise<ContextSource | null>;
  createContextSource(payload: CreateContextSourcePayload): Promise<ContextSource>;
  updateContextSource(payload: UpdateContextSourcePayload): Promise<ContextSource>;
  deleteContextSource(contextSourceId: string): Promise<void>;
}
