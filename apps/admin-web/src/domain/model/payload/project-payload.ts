export interface CreateProjectPayload {
  name: string;
  description: string;
  platform: "android" | "ios" | "web" | "multi";
  repositoryUrl: string;
  directoryPath: string;
  ownerId?: string | null;
  status?: string;
  artifactStoragePreference?: "supabase" | "google_drive";
  defaultProvider?: string | null;
  defaultModel?: string | null;
  defaultReasoningEffort?: string | null;
  sessionIdleTtlMinutes?: number | null;
}

export interface UpdateProjectPayload {
  name?: string;
  description?: string;
  platform?: "android" | "ios" | "web" | "multi";
  repositoryUrl?: string;
  directoryPath?: string | null;
  ownerId?: string | null;
  status?: string;
  artifactStoragePreference?: "supabase" | "google_drive";
  defaultProvider?: string | null;
  defaultModel?: string | null;
  defaultReasoningEffort?: string | null;
  sessionIdleTtlMinutes?: number | null;
}
