export type IntegrationType =
  | "jira"
  | "figma"
  | "google_drive"
  | "firebase"
  | "telegram";

export type IntegrationStatus =
  | "pending"
  | "awaiting_oauth"
  | "connected"
  | "failed";

export interface Integration {
  id: string;
  projectId: string;
  type: IntegrationType;
  label: string;
  mcpTypeEnabled?: boolean;
  configEncrypted: Record<string, unknown>;
  status: IntegrationStatus;
  lastSyncedAt: string | null;
  lastError: string | null;
  createdAt: string;
  updatedAt: string;
}
