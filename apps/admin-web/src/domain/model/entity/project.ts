export interface Project {
  id: string;
  name: string;
  description: string;
  platform: "android" | "ios" | "web" | "multi";
  repositoryUrl: string;
  directoryPath: string | null;
  ownerId: string | null;
  status: string;
  artifactStoragePreference: "supabase" | "google_drive";
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}
