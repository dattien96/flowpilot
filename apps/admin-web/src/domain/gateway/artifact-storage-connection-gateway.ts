import type { ArtifactStorageProvider } from "./artifact-storage-gateway";

export type ArtifactStorageSyncStatus = "local_only" | "queued" | "syncing" | "synced" | "failed";

export interface ArtifactRunStorageMetadataUpdate {
  artifactRunId: string;
  storageProvider?: ArtifactStorageProvider | null;
  remotePath?: string | null;
  remoteObjectId?: string | null;
  syncStatus?: ArtifactStorageSyncStatus;
}

export interface ArtifactStorageConnectionGateway {
  getProjectStorageProvider(projectId: string): Promise<ArtifactStorageProvider>;
  updateArtifactRunStorageMetadata(update: ArtifactRunStorageMetadataUpdate): Promise<void>;
}
