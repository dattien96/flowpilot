import type {
  IntegrationStatus,
  IntegrationType,
} from "@/domain/model/entity/integration";

export interface CreateIntegrationPayload {
  projectId: string;
  type: IntegrationType;
  label: string;
  configEncrypted: Record<string, unknown>;
  status?: IntegrationStatus;
  mcpTypeEnabled?: boolean;
}

export interface UpdateIntegrationPayload {
  type: IntegrationType;
  label: string;
  configEncrypted: Record<string, unknown>;
  status?: IntegrationStatus;
  lastSyncedAt?: string | null;
  lastError?: string | null;
  mcpTypeEnabled?: boolean;
}
