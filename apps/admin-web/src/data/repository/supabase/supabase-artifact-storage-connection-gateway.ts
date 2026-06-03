import type { SupabaseClient } from "@supabase/supabase-js";

import type {
  ArtifactRunStorageMetadataUpdate,
  ArtifactStorageConnectionGateway,
} from "@/domain/gateway/artifact-storage-connection-gateway";
import type { ArtifactStorageProvider } from "@/domain/gateway/artifact-storage-gateway";

function normalizeProvider(value: unknown): ArtifactStorageProvider {
  return value === "google_drive" ? "google_drive" : "supabase";
}

function buildArtifactRunPatch(update: ArtifactRunStorageMetadataUpdate) {
  const patch: Record<string, unknown> = {
    updated_at: new Date().toISOString(),
  };

  if (update.storageProvider !== undefined) {
    patch.storage_provider = update.storageProvider;
  }
  if (update.remotePath !== undefined) {
    patch.remote_path = update.remotePath;
  }
  if (update.remoteObjectId !== undefined) {
    patch.remote_object_id = update.remoteObjectId;
  }
  if (update.syncStatus !== undefined) {
    patch.sync_status = update.syncStatus;
  }

  return patch;
}

export class SupabaseArtifactStorageConnectionGateway
  implements ArtifactStorageConnectionGateway
{
  constructor(private readonly supabase: SupabaseClient) {}

  async getProjectStorageProvider(projectId: string): Promise<ArtifactStorageProvider> {
    const trimmedProjectId = projectId.trim();
    if (!trimmedProjectId) {
      return "supabase";
    }

    const { data, error } = await this.supabase
      .from("projects")
      .select("artifact_storage_preference")
      .eq("id", trimmedProjectId)
      .maybeSingle();

    if (error || !data) {
      return "supabase";
    }

    return normalizeProvider(data.artifact_storage_preference);
  }

  async updateArtifactRunStorageMetadata(update: ArtifactRunStorageMetadataUpdate): Promise<void> {
    const artifactRunId = update.artifactRunId.trim();
    if (!artifactRunId) {
      return;
    }

    const patch = buildArtifactRunPatch(update);
    const { data, error } = await this.supabase
      .from("artifact_runs")
      .update(patch)
      .eq("id", artifactRunId)
      .select("id")
      .maybeSingle();

    if (error) {
      throw new Error(error.message);
    }
    if (!data) {
      throw new Error(
        `Artifact run "${artifactRunId}" was not found in shared sync state. Only workflow-backed artifacts can appear in Remote/Synced across PCs.`,
      );
    }
  }
}
