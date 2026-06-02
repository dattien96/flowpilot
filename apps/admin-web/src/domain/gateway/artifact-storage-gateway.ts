export type ArtifactStorageProvider = "supabase" | "google_drive";

export interface ArtifactStorageFile {
  relativePath: string;
  bytes: Uint8Array;
}

export interface ArtifactStorageSyncRequest {
  projectId: string;
  artifactId: string;
  workflowRunId: string;
  workflowStepKey: string;
  outputFilename: string;
  createdAt: string;
  files: ArtifactStorageFile[];
}

export interface ArtifactStorageSyncResult {
  storageProvider: ArtifactStorageProvider;
  remotePath: string;
  remoteObjectId?: string | null;
}

export interface ArtifactStorageGateway {
  syncSnapshot(request: ArtifactStorageSyncRequest): Promise<ArtifactStorageSyncResult>;
  createOpenUrl(remotePath: string, remoteObjectId?: string | null): Promise<string>;
}
