import type { Integration } from "@/domain/model/entity/integration";
import type {
  CreateIntegrationPayload,
  UpdateIntegrationPayload,
} from "@/domain/model/payload/integration-payload";

export interface IntegrationGateway {
  listIntegrationsByProject(projectId: string): Promise<Integration[]>;
  listAllIntegrations(): Promise<Integration[]>;
  listLinkedIntegrationsByProject(projectId: string): Promise<Integration[]>;
  createIntegration(payload: CreateIntegrationPayload): Promise<Integration>;
  updateIntegration(
    integrationId: string,
    patch: Partial<UpdateIntegrationPayload>,
  ): Promise<Integration>;
  deleteIntegration(integrationId: string): Promise<void>;
  linkIntegrationToProject(projectId: string, integrationId: string): Promise<void>;
  unlinkIntegrationFromProject(projectId: string, integrationId: string): Promise<void>;
}
