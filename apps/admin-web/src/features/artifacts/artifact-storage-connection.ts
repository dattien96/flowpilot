import type { SupabaseClient } from "@supabase/supabase-js";

export type ArtifactStorageProvider = "supabase" | "google_drive";

export type ArtifactStorageConnectionStatus =
  | "disconnected"
  | "pending"
  | "connected"
  | "reconnect_required"
  | "failed";

export interface ArtifactStorageConnectionRecord {
  projectId: string;
  provider: ArtifactStorageProvider;
  status: ArtifactStorageConnectionStatus;
  folderId: string;
  folderName: string;
  oauthAccountEmail: string;
  lastValidatedAt: string;
  lastError: string;
  connectedAt: string;
  updatedAt: string;
}

function normalizeProvider(value: unknown): ArtifactStorageProvider {
  return value === "google_drive" ? "google_drive" : "supabase";
}

function normalizeStatus(value: unknown): ArtifactStorageConnectionStatus {
  switch (value) {
    case "pending":
    case "connected":
    case "reconnect_required":
    case "failed":
      return value;
    default:
      return "disconnected";
  }
}

function mapArtifactStorageConnection(row: Record<string, unknown>): ArtifactStorageConnectionRecord {
  return {
    projectId: String(row.project_id ?? ""),
    provider: normalizeProvider(row.provider),
    status: normalizeStatus(row.status),
    folderId: String(row.folder_id ?? ""),
    folderName: String(row.folder_name ?? ""),
    oauthAccountEmail: String(row.oauth_account_email ?? ""),
    lastValidatedAt: String(row.last_validated_at ?? ""),
    lastError: String(row.last_error ?? ""),
    connectedAt: String(row.connected_at ?? ""),
    updatedAt: String(row.updated_at ?? ""),
  };
}

export async function getArtifactStorageConnection(
  supabase: SupabaseClient,
  projectId: string,
  provider: ArtifactStorageProvider,
): Promise<ArtifactStorageConnectionRecord | null> {
  const { data, error } = await supabase
    .from("artifact_storage_connections")
    .select("*")
    .eq("project_id", projectId.trim())
    .eq("provider", provider)
    .maybeSingle();

  if (error || !data) {
    return null;
  }

  return mapArtifactStorageConnection(data as Record<string, unknown>);
}

export async function upsertArtifactStorageConnection(
  supabase: SupabaseClient,
  update: {
    projectId: string;
    provider: ArtifactStorageProvider;
    status: ArtifactStorageConnectionStatus;
    folderId?: string | null;
    folderName?: string | null;
    oauthAccountEmail?: string | null;
    lastValidatedAt?: string | null;
    lastError?: string | null;
    connectedAt?: string | null;
  },
) {
  const patch = {
    project_id: update.projectId.trim(),
    provider: update.provider,
    status: update.status,
    folder_id: update.folderId?.trim() || null,
    folder_name: update.folderName?.trim() || null,
    oauth_account_email: update.oauthAccountEmail?.trim() || null,
    last_validated_at: update.lastValidatedAt?.trim() || null,
    last_error: update.lastError?.trim() || null,
    connected_at: update.connectedAt?.trim() || null,
    updated_at: new Date().toISOString(),
  };

  const { error } = await supabase
    .from("artifact_storage_connections")
    .upsert(patch, { onConflict: "project_id,provider" });

  if (error) {
    throw new Error(error.message);
  }
}

export function isArtifactStorageConnectionReady(
  connection: Pick<ArtifactStorageConnectionRecord, "status" | "folderId"> | null,
) {
  return Boolean(connection && connection.status === "connected" && connection.folderId.trim());
}
