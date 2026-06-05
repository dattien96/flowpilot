import type { SupabaseClient } from "@supabase/supabase-js";

import type {
  ArtifactRunStorageMetadataUpdate,
  ArtifactStorageConnectionGateway,
} from "@/domain/gateway/artifact-storage-connection-gateway";
import type { ArtifactStorageProvider } from "@/domain/gateway/artifact-storage-gateway";
import type { ArtifactRunReplica } from "@/domain/model/entity/workflow-engine";

const ARTIFACT_SUPABASE_BUCKET_NAME = "flowpilot-artifacts";

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

function normalizeReplicaSyncStatus(
  value: ArtifactRunStorageMetadataUpdate["syncStatus"],
): ArtifactRunReplica["syncStatus"] {
  switch (value) {
    case "syncing":
    case "synced":
    case "failed":
      return value;
    default:
      return "queued";
  }
}

function mapArtifactRunReplica(row: Record<string, unknown>): ArtifactRunReplica {
  return {
    id: String(row.id ?? ""),
    artifactRunId: String(row.artifact_run_id ?? ""),
    projectId: row.project_id ? String(row.project_id) : null,
    provider: row.provider === "google_drive" ? "google_drive" : "supabase",
    storageScopeKey: row.storage_scope_key ? String(row.storage_scope_key) : null,
    remotePath: String(row.remote_path ?? ""),
    remoteObjectId: row.remote_object_id ? String(row.remote_object_id) : null,
    syncStatus: normalizeReplicaSyncStatus(String(row.sync_status ?? "queued") as ArtifactRunStorageMetadataUpdate["syncStatus"]),
    checksum: row.checksum ? String(row.checksum) : null,
    lastSyncedAt: row.last_synced_at ? String(row.last_synced_at) : null,
    lastError: row.last_error ? String(row.last_error) : null,
    createdAt: row.created_at ? String(row.created_at) : "",
    updatedAt: row.updated_at ? String(row.updated_at) : "",
  };
}

function deriveSupabaseStorageScopeKey() {
  const baseUrl = process.env.SUPABASE_API_URL?.trim().replace(/\/+$/, "") ?? "";
  if (!baseUrl) {
    return null;
  }

  return `supabase:${baseUrl}:${ARTIFACT_SUPABASE_BUCKET_NAME}`;
}

export class SupabaseArtifactStorageConnectionGateway
  implements ArtifactStorageConnectionGateway
{
  constructor(private readonly supabase: SupabaseClient) {}

  private async deriveStorageScopeKey(
    projectId: string,
    provider: ArtifactStorageProvider,
  ): Promise<string | null> {
    if (provider === "supabase") {
      return deriveSupabaseStorageScopeKey();
    }

    const { data, error } = await this.supabase
      .from("artifact_storage_connections")
      .select("folder_id")
      .eq("project_id", projectId)
      .eq("provider", "google_drive")
      .maybeSingle();

    if (error) {
      throw new Error(error.message);
    }

    const folderId = String(data?.folder_id ?? "").trim();
    return folderId ? `gdrive:${folderId}` : null;
  }

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
      .select("id, project_id")
      .maybeSingle();

    if (error) {
      throw new Error(error.message);
    }
    if (!data) {
      throw new Error(
        `Artifact run "${artifactRunId}" was not found in shared sync state. Only workflow-backed artifacts can appear in Remote/Synced across PCs.`,
      );
    }

    const provider = update.storageProvider ?? null;
    const projectId = String(data.project_id ?? "").trim();
    if (!provider || !projectId) {
      return;
    }
    const storageScopeKey =
      update.storageScopeKey === undefined
        ? await this.deriveStorageScopeKey(projectId, provider)
        : update.storageScopeKey;

    const replicaPatch = {
      artifact_run_id: artifactRunId,
      project_id: projectId,
      provider,
      storage_scope_key: storageScopeKey,
      remote_path: update.remotePath ?? "",
      remote_object_id: update.remoteObjectId ?? null,
      sync_status: normalizeReplicaSyncStatus(update.syncStatus),
      checksum: update.checksum ?? null,
      last_synced_at: update.syncStatus === "synced" ? new Date().toISOString() : null,
      last_error: update.lastError ?? null,
      updated_at: new Date().toISOString(),
    };

    const { error: replicaError } = await this.supabase
      .from("artifact_run_replicas")
      .upsert(replicaPatch, { onConflict: "artifact_run_id,provider" });

    if (replicaError) {
      throw new Error(replicaError.message);
    }
  }

  async listArtifactRunReplicas(artifactRunIds: string[]): Promise<Record<string, ArtifactRunReplica[]>> {
    const ids = [...new Set(artifactRunIds.map((id) => id.trim()).filter(Boolean))];
    if (ids.length === 0) {
      return {};
    }

    const { data, error } = await this.supabase
      .from("artifact_run_replicas")
      .select("*")
      .in("artifact_run_id", ids)
      .order("updated_at", { ascending: false });

    if (error) {
      throw new Error(error.message);
    }

    const grouped: Record<string, ArtifactRunReplica[]> = {};
    for (const row of (data ?? []) as Record<string, unknown>[]) {
      const replica = mapArtifactRunReplica(row);
      if (!grouped[replica.artifactRunId]) {
        grouped[replica.artifactRunId] = [];
      }
      grouped[replica.artifactRunId].push(replica);
    }

    return grouped;
  }
}
