import type { ArtifactRunReplica } from "@/domain/model/entity/workflow-engine";
import type { ArtifactStorageProvider } from "./artifact-storage-gateway";

export type ArtifactStorageSyncStatus = "local_only" | "queued" | "syncing" | "synced" | "failed";

export interface ArtifactRunStorageMetadataUpdate {
  artifactRunId: string;
  storageProvider?: ArtifactStorageProvider | null;
  storageScopeKey?: string | null;
  remotePath?: string | null;
  remoteObjectId?: string | null;
  syncStatus?: ArtifactStorageSyncStatus;
  checksum?: string | null;
  lastError?: string | null;
}

export interface ArtifactStorageConnectionGateway {
  getProjectStorageProvider(projectId: string): Promise<ArtifactStorageProvider>;
  updateArtifactRunStorageMetadata(update: ArtifactRunStorageMetadataUpdate): Promise<void>;
  listArtifactRunReplicas(artifactRunIds: string[]): Promise<Record<string, ArtifactRunReplica[]>>;
}
