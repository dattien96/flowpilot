export interface CreateProjectPayload {
  name: string;
  description: string;
  platform: "android" | "ios" | "web" | "multi";
  repositoryUrl: string;
  directoryPath?: string | null;
  ownerId?: string | null;
  status?: string;
  artifactStoragePreference?: "supabase" | "google_drive";
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
}
