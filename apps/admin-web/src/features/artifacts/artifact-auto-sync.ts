import type { SupabaseClient } from "@supabase/supabase-js";

import type { ArtifactStorageConnectionGateway } from "@/domain/gateway/artifact-storage-connection-gateway";
import type { ArtifactStorageProvider } from "@/domain/gateway/artifact-storage-gateway";
import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";
import {
  getArtifactStorageConnection,
  isArtifactStorageConnectionReady,
} from "@/features/artifacts/artifact-storage-connection";

const ARTIFACT_SYNC_DEBOUNCE_MS = 5 * 60 * 1000;
const ARTIFACT_SYNC_STALE_MS = ARTIFACT_SYNC_DEBOUNCE_MS;

type ArtifactSyncRow = {
  id: string;
  project_id: string | null;
  workflow_run_id: string | null;
  sync_status: string | null;
  storage_provider: ArtifactStorageProvider | null;
  remote_path: string | null;
  remote_object_id: string | null;
  updated_at: string | null;
};

type ArtifactSyncContext = {
  supabase: SupabaseClient;
  artifactStorageConnectionGateway: ArtifactStorageConnectionGateway;
};

type ArtifactStorageSyncRequest = {
  storageProvider: ArtifactStorageProvider;
  googleDriveProjectId?: string;
  googleDriveFolderId?: string;
};

type ArtifactSyncResult = {
  status: "synced" | "failed" | "skipped";
  artifactRunId: string;
  reason?: string;
};

const scheduledSyncTimers = new Map<string, ReturnType<typeof setTimeout>>();
const activeSyncs = new Map<string, Promise<ArtifactSyncResult>>();
let bootstrapSyncStarted = false;

function isStaleSync(row: ArtifactSyncRow) {
  if (row.sync_status !== "syncing" || !row.updated_at) {
    return false;
  }

  const updatedAt = Date.parse(row.updated_at);
  return Number.isFinite(updatedAt) && Date.now() - updatedAt >= ARTIFACT_SYNC_STALE_MS;
}

function needsSync(row: ArtifactSyncRow) {
  if (row.sync_status === "syncing") {
    return isStaleSync(row);
  }

  if (row.sync_status !== "synced") {
    return true;
  }

  return !row.storage_provider || !row.remote_path || !row.remote_object_id;
}

async function listArtifactSyncCandidates(
  supabase: SupabaseClient,
  filters?: { projectId?: string; workflowRunId?: string },
) {
  let query = supabase
    .from("artifact_runs")
    .select(
      "id, project_id, workflow_run_id, sync_status, storage_provider, remote_path, remote_object_id, updated_at",
    )
    .order("updated_at", { ascending: true });

  if (filters?.projectId) {
    query = query.eq("project_id", filters.projectId);
  }

  if (filters?.workflowRunId) {
    query = query.eq("workflow_run_id", filters.workflowRunId);
  }

  const { data, error } = await query;
  if (error) {
    throw new Error(`Unable to list artifact sync candidates: ${error.message}`);
  }

  return ((data ?? []) as ArtifactSyncRow[]).filter(needsSync);
}

export async function resolveArtifactStorageSyncRequest(
  context: ArtifactSyncContext,
  projectId: string,
): Promise<ArtifactStorageSyncRequest | null> {
  const storageProvider =
    await context.artifactStorageConnectionGateway.getProjectStorageProvider(projectId);

  if (storageProvider !== "google_drive") {
    return { storageProvider };
  }

  const connection = await getArtifactStorageConnection(
    context.supabase,
    projectId,
    "google_drive",
  );
  if (!connection || !isArtifactStorageConnectionReady(connection)) {
    return null;
  }

  return {
    storageProvider,
    googleDriveProjectId: projectId,
    googleDriveFolderId: connection.folderId,
  };
}

async function persistArtifactSyncState(
  context: ArtifactSyncContext,
  artifactRunId: string,
  update: Partial<{
    storageProvider: ArtifactStorageProvider;
    remotePath: string | null;
    remoteObjectId: string | null;
    syncStatus: "local_only" | "queued" | "syncing" | "synced" | "failed";
  }>,
) {
  await context.artifactStorageConnectionGateway.updateArtifactRunStorageMetadata({
    artifactRunId,
    storageProvider: update.storageProvider,
    remotePath: update.remotePath ?? undefined,
    remoteObjectId: update.remoteObjectId ?? undefined,
    syncStatus: update.syncStatus,
  });
}

async function callLocalRunnerArtifactSync(
  artifactRunId: string,
  request: ArtifactStorageSyncRequest,
) {
  const response = await fetch(new URL(`/artifacts/${artifactRunId}/sync`, getLocalRunnerBaseUrl()), {
    method: "POST",
    cache: "no-store",
    headers: {
      "content-type": "application/json",
    },
    body: JSON.stringify(request),
  });

  const payload = (await response.json().catch(() => null)) as
    | {
        error?: string;
        storageProvider?: ArtifactStorageProvider | string;
        remotePath?: string;
        remoteObjectId?: string | null;
        syncStatus?: "local_only" | "queued" | "syncing" | "synced" | "failed";
      }
    | null;

  if (!response.ok) {
    throw new Error(payload?.error ?? `Artifact sync failed (${response.status}).`);
  }

  return payload;
}

async function performArtifactSync(
  context: ArtifactSyncContext,
  artifactRun: ArtifactSyncRow,
): Promise<ArtifactSyncResult> {
  const syncRequest = await resolveArtifactStorageSyncRequest(
    context,
    String(artifactRun.project_id ?? ""),
  );

  if (!syncRequest) {
    return {
      status: "skipped",
      artifactRunId: artifactRun.id,
      reason: "provider_not_ready",
    };
  }

  await persistArtifactSyncState(context, artifactRun.id, {
    storageProvider: syncRequest.storageProvider,
    syncStatus: "syncing",
  });

  try {
    const payload = await callLocalRunnerArtifactSync(artifactRun.id, syncRequest);
    const storageProvider =
      payload?.storageProvider === "google_drive"
        ? "google_drive"
        : syncRequest.storageProvider;
    const syncStatus = payload?.syncStatus === "failed" ? "failed" : "synced";

    await persistArtifactSyncState(context, artifactRun.id, {
      storageProvider,
      remotePath: payload?.remotePath ?? null,
      remoteObjectId: payload?.remoteObjectId ?? null,
      syncStatus,
    });

    return {
      status: syncStatus,
      artifactRunId: artifactRun.id,
    };
  } catch (error) {
    await persistArtifactSyncState(context, artifactRun.id, {
      storageProvider: syncRequest.storageProvider,
      syncStatus: "failed",
    });
    throw error;
  }
}

export async function syncArtifactRun(
  context: ArtifactSyncContext,
  artifactRun: ArtifactSyncRow,
) {
  const existing = activeSyncs.get(artifactRun.id);
  if (existing) {
    return existing;
  }

  const promise = performArtifactSync(context, artifactRun).finally(() => {
    activeSyncs.delete(artifactRun.id);
  });
  activeSyncs.set(artifactRun.id, promise);
  return promise;
}

export async function reconcileArtifactSyncState(
  context: ArtifactSyncContext,
  filters?: { projectId?: string; workflowRunId?: string },
) {
  const candidates = await listArtifactSyncCandidates(context.supabase, filters);
  console.info(
    `[artifact-sync] reconcile candidates=${candidates.length}` +
      (filters?.projectId ? ` project_id=${filters.projectId}` : "") +
      (filters?.workflowRunId ? ` workflow_run_id=${filters.workflowRunId}` : ""),
  );
  const results = await Promise.allSettled(
    candidates.map((artifactRun) => syncArtifactRun(context, artifactRun)),
  );
  const failedCount = results.filter((result) => result.status === "rejected").length;
  if (failedCount > 0) {
    console.warn(`[artifact-sync] reconcile failures=${failedCount}`);
  }
}

export function scheduleArtifactAutoSync(
  context: ArtifactSyncContext,
  filters: { projectId: string; workflowRunId: string },
) {
  if (
    !context ||
    typeof context !== "object" ||
    !("supabase" in context) ||
    !context.supabase ||
    typeof (context.supabase as { from?: unknown }).from !== "function"
  ) {
    throw new Error("Artifact auto-sync requires a valid sync context.");
  }

  const key = `${filters.projectId}:${filters.workflowRunId}`;
  const previous = scheduledSyncTimers.get(key);
  if (previous) {
    clearTimeout(previous);
  }

  const timer = setTimeout(() => {
    scheduledSyncTimers.delete(key);
    void reconcileArtifactSyncState(context, filters).catch((error) => {
      console.warn("Artifact auto-sync reconciliation failed:", error);
    });
  }, ARTIFACT_SYNC_DEBOUNCE_MS);

  scheduledSyncTimers.set(key, timer);
}

export function startArtifactSyncBootstrap(context: ArtifactSyncContext) {
  if (bootstrapSyncStarted) {
    return;
  }

  bootstrapSyncStarted = true;
  triggerArtifactSyncBootstrap(context);
}

export function triggerArtifactSyncBootstrap(context: ArtifactSyncContext) {
  void reconcileArtifactSyncState(context).catch((error) => {
    console.warn("Artifact sync bootstrap reconciliation failed:", error);
  });
}

export function resetArtifactAutoSyncState() {
  for (const timer of scheduledSyncTimers.values()) {
    clearTimeout(timer);
  }
  scheduledSyncTimers.clear();
  activeSyncs.clear();
  bootstrapSyncStarted = false;
}
