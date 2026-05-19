export interface Integration {
  id: string;
  projectId: string;
  type: "jira" | "figma" | "google_drive" | "firebase" | "telegram";
  configEncrypted: Record<string, unknown>;
  status: "pending" | "connected" | "failed";
  lastSyncedAt: string | null;
  createdAt: string;
  updatedAt: string;
}
