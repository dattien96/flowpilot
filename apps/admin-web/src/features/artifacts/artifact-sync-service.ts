import type { SupabaseClient } from "@supabase/supabase-js";

import type { ArtifactStorageConnectionGateway } from "@/domain/gateway/artifact-storage-connection-gateway";
import type { ArtifactStorageProvider } from "@/domain/gateway/artifact-storage-gateway";
import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";
import { SupabaseArtifactStorageGateway } from "@/data/repository/supabase/supabase-artifact-storage-gateway";
import { decodeArtifactSyncBundle } from "@/features/artifacts/decode-artifact-sync-bundle";
import { getArtifactStorageConnection } from "@/features/artifacts/artifact-storage-connection";
import { getLocalRunnerBaseUrl } from "@/lib/env/app-env";

type ArtifactStorageConnection = NonNullable<
  Awaited<ReturnType<typeof getArtifactStorageConnection>>
>;

type ArtifactSyncDependencies = {
  supabase: SupabaseClient;
  localRunnerGateway: Pick<
    LocalRunnerGateway,
    "getArtifactById" | "listArtifacts" | "exportArtifactSyncBundle" | "saveArtifactCloudSyncResult"
  >;
  artifactStorageConnectionGateway: Pick<
    ArtifactStorageConnectionGateway,
    "updateArtifactRunStorageMetadata"
  >;
};

type SyncArtifactOptions = {
  targetProvider?: ArtifactStorageProvider;
};

type ArtifactSyncRequest = {
  storageProvider: ArtifactStorageProvider;
  googleDriveProjectId?: string;
  googleDriveFolderId?: string;
};

type ArtifactSyncResult = {
  storageProvider?: string;
  remotePath?: string;
  remoteObjectId?: string | null;
  syncStatus?: string;
  error?: string;
};

const autoSyncTimers = new Map<string, ReturnType<typeof setTimeout>>();
let autoSyncBootstrapRequested = false;

function toArtifactSyncBody(request: ArtifactSyncRequest) {
  return JSON.stringify(request);
}

async function loadProjectStorageProvider(
  supabase: SupabaseClient,
  projectId: string,
) {
  const { data, error } = await supabase
    .from("projects")
    .select("artifact_storage_preference")
    .eq("id", projectId.trim())
    .maybeSingle();

  if (error || !data) {
    return "supabase" as const;
  }

  return (data.artifact_storage_preference === "google_drive"
    ? "google_drive"
    : "supabase") as ArtifactStorageProvider;
}

async function loadGoogleDriveConnection(
  supabase: SupabaseClient,
  projectId: string,
): Promise<ArtifactStorageConnection | null> {
  return getArtifactStorageConnection(supabase, projectId, "google_drive");
}

async function updateArtifactSyncState(
  gateway: Pick<ArtifactStorageConnectionGateway, "updateArtifactRunStorageMetadata">,
  artifactId: string,
  patch: {
    storageProvider?: ArtifactStorageProvider | null;
    remotePath?: string | null;
    remoteObjectId?: string | null;
    syncStatus?: ArtifactSyncResult["syncStatus"];
    checksum?: string | null;
    lastError?: string | null;
  },
) {
  await gateway.updateArtifactRunStorageMetadata({
    artifactRunId: artifactId,
    storageProvider: patch.storageProvider ?? undefined,
    remotePath: patch.remotePath ?? undefined,
    remoteObjectId: patch.remoteObjectId ?? undefined,
    syncStatus: patch.syncStatus as "local_only" | "queued" | "syncing" | "synced" | "failed" | undefined,
    checksum: patch.checksum ?? undefined,
    lastError: patch.lastError ?? undefined,
  });
}

async function tryUpdateArtifactSyncState(
  gateway: Pick<ArtifactStorageConnectionGateway, "updateArtifactRunStorageMetadata">,
  artifactId: string,
  patch: Parameters<typeof updateArtifactSyncState>[2],
) {
  try {
    await updateArtifactSyncState(gateway, artifactId, patch);
  } catch (error) {
    console.warn(
      `Unable to mirror artifact sync metadata for ${artifactId}:`,
      error instanceof Error ? error.message : error,
    );
  }
}

async function markLocalArtifactSyncFailed(
  dependencies: ArtifactSyncDependencies,
  artifactId: string,
  patch: {
    storageProvider?: ArtifactStorageProvider;
    remotePath?: string | null;
    remoteObjectId?: string | null;
    errorMessage: string;
  },
) {
  try {
    await dependencies.localRunnerGateway.saveArtifactCloudSyncResult(artifactId, {
      storageProvider: patch.storageProvider ?? "",
      remotePath: patch.remotePath ?? "",
      remoteObjectId: patch.remoteObjectId ?? null,
      syncStatus: "failed",
      errorMessage: patch.errorMessage,
    });
  } catch (error) {
    console.warn(
      `Unable to persist local failed sync metadata for ${artifactId}:`,
      error instanceof Error ? error.message : error,
    );
  }
}

function deriveOutputFilename(artifact: Pick<LocalRunnerArtifact, "contentPath" | "title" | "artifactId">) {
  const candidate = artifact.contentPath?.trim() || artifact.title.trim() || `${artifact.artifactId}.md`;
  const normalized = candidate.replaceAll("\\", "/");
  const base = normalized.split("/").filter(Boolean).at(-1) || "content.md";
  return /\.[a-z0-9]+$/i.test(base) ? base : `${base}.md`;
}

async function syncArtifactToSupabase(
  artifact: LocalRunnerArtifact,
  dependencies: ArtifactSyncDependencies,
) {
  const bundle = await dependencies.localRunnerGateway.exportArtifactSyncBundle(artifact.artifactId);
  const files = decodeArtifactSyncBundle(bundle);
  const gateway = new SupabaseArtifactStorageGateway(dependencies.supabase);
  const outputFilename = deriveOutputFilename(artifact);
  const createdAt = artifact.createdAt.trim() || artifact.updatedAt.trim() || new Date().toISOString();

  const result = await gateway.syncSnapshot({
    projectId: artifact.projectId,
    artifactId: artifact.artifactId,
    workflowRunId: artifact.workflowRunId,
    workflowStepKey: artifact.workflowStepKey,
    outputFilename,
    createdAt,
    files,
  });

  await dependencies.localRunnerGateway.saveArtifactCloudSyncResult(artifact.artifactId, {
    storageProvider: result.storageProvider,
    remotePath: result.remotePath,
    remoteObjectId: result.remoteObjectId ?? null,
    syncStatus: "synced",
  });

  return {
    storageProvider: result.storageProvider,
    remotePath: result.remotePath,
    remoteObjectId: result.remoteObjectId ?? null,
    syncStatus: "synced",
  } satisfies ArtifactSyncResult;
}

export async function syncArtifactThroughRunner(
  artifactId: string,
  dependencies: ArtifactSyncDependencies,
  options: SyncArtifactOptions = {},
) {
  const artifact = await dependencies.localRunnerGateway.getArtifactById(artifactId);
  if (!artifact) {
    throw new Error("Artifact not found.");
  }

  const projectId = artifact.projectId.trim();
  if (!projectId) {
    throw new Error("Artifact is missing its project id.");
  }

  const projectProvider = options.targetProvider ?? (await loadProjectStorageProvider(dependencies.supabase, projectId));
  let googleDriveConnection: ArtifactStorageConnection | null = null;

  if (projectProvider === "google_drive") {
    googleDriveConnection = await loadGoogleDriveConnection(dependencies.supabase, projectId);
    if (
      !googleDriveConnection ||
      googleDriveConnection.status !== "connected" ||
      !googleDriveConnection.folderId.trim()
    ) {
      const error = new Error(
        "Google Drive artifact storage is not connected on this runner for the selected project.",
      );
      throw error;
    }
  }

  await updateArtifactSyncState(dependencies.artifactStorageConnectionGateway, artifactId, {
    storageProvider: projectProvider,
    syncStatus: "syncing",
    checksum: artifact.checksum ?? null,
  });

  if (projectProvider === "supabase") {
    let payload: ArtifactSyncResult | null = null;
    try {
      payload = await syncArtifactToSupabase(artifact, dependencies);
      await updateArtifactSyncState(dependencies.artifactStorageConnectionGateway, artifactId, {
        storageProvider: "supabase",
        remotePath: payload.remotePath ?? artifact.remotePath ?? "",
        remoteObjectId: payload.remoteObjectId ?? artifact.remoteObjectId ?? null,
        syncStatus: "synced",
        checksum: artifact.checksum ?? null,
        lastError: null,
      });
      return payload;
    } catch (error) {
      const message = error instanceof Error ? error.message : "Unable to sync artifact.";
      if (payload) {
        await markLocalArtifactSyncFailed(dependencies, artifact.artifactId, {
          storageProvider: "supabase",
          remotePath: payload.remotePath ?? artifact.remotePath ?? "",
          remoteObjectId: payload.remoteObjectId ?? artifact.remoteObjectId ?? null,
          errorMessage: message,
        });
      }
      await tryUpdateArtifactSyncState(dependencies.artifactStorageConnectionGateway, artifactId, {
        storageProvider: "supabase",
        syncStatus: "failed",
        checksum: artifact.checksum ?? null,
        lastError: message,
      });
      throw error instanceof Error ? error : new Error("Unable to sync artifact.");
    }
  }

  let response: Response;
  try {
    response = await fetch(new URL(`/artifacts/${artifactId}/sync`, getLocalRunnerBaseUrl()), {
      method: "POST",
      cache: "no-store",
      headers: {
        "content-type": "application/json",
      },
      body:
        projectProvider === "google_drive" && googleDriveConnection
          ? toArtifactSyncBody({
              storageProvider: projectProvider,
              googleDriveProjectId: projectId,
              googleDriveFolderId: googleDriveConnection.folderId,
            })
          : toArtifactSyncBody({ storageProvider: projectProvider }),
    });
  } catch (error) {
    await tryUpdateArtifactSyncState(dependencies.artifactStorageConnectionGateway, artifactId, {
      storageProvider: projectProvider,
      syncStatus: "failed",
      checksum: artifact.checksum ?? null,
      lastError: error instanceof Error ? error.message : "Unable to sync artifact.",
    });
    throw error instanceof Error ? error : new Error("Unable to sync artifact.");
  }

  const payload = (await response.json().catch(() => null)) as ArtifactSyncResult | null;
  if (!response.ok) {
    await tryUpdateArtifactSyncState(dependencies.artifactStorageConnectionGateway, artifactId, {
      storageProvider: projectProvider,
      syncStatus: "failed",
      checksum: artifact.checksum ?? null,
      lastError:
        payload?.error ||
        (payload && "message" in payload ? String((payload as { message?: string }).message) : "") ||
        "Unable to sync artifact.",
    });
    throw new Error(
      payload?.error ||
        (payload && "message" in payload ? String((payload as { message?: string }).message) : "") ||
        "Unable to sync artifact.",
    );
  }

  const resolvedProvider =
    payload?.storageProvider === "google_drive" ? "google_drive" : projectProvider;

  try {
    await updateArtifactSyncState(dependencies.artifactStorageConnectionGateway, artifactId, {
      storageProvider: resolvedProvider,
      remotePath: payload?.remotePath ?? artifact.remotePath ?? "",
      remoteObjectId: payload?.remoteObjectId ?? artifact.remoteObjectId ?? null,
      syncStatus: "synced",
      checksum: artifact.checksum ?? null,
      lastError: null,
    });
    return payload;
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unable to sync artifact.";
    await markLocalArtifactSyncFailed(dependencies, artifact.artifactId, {
      storageProvider: resolvedProvider,
      remotePath: payload?.remotePath ?? artifact.remotePath ?? "",
      remoteObjectId: payload?.remoteObjectId ?? artifact.remoteObjectId ?? null,
      errorMessage: message,
    });
    await tryUpdateArtifactSyncState(dependencies.artifactStorageConnectionGateway, artifactId, {
      storageProvider: resolvedProvider,
      syncStatus: "failed",
      checksum: artifact.checksum ?? null,
      lastError: message,
    });
    throw error instanceof Error ? error : new Error("Unable to sync artifact.");
  }
}

async function syncEligibleArtifacts(
  dependencies: ArtifactSyncDependencies,
  artifacts: LocalRunnerArtifact[],
) {
  for (const artifact of artifacts) {
    if (artifact.syncStatus === "synced") {
      continue;
    }

    try {
      await syncArtifactThroughRunner(artifact.artifactId, dependencies);
    } catch {
      // Best-effort reconciliation. The sync route already persists failure state.
    }
  }
}

export function scheduleArtifactAutoSync(
  scope: {
    projectId: string;
    workflowRunId: string;
    artifactIds: string[];
  },
  dependencies: ArtifactSyncDependencies,
  delayMs = 5 * 60 * 1000,
) {
  const key = `${scope.projectId.trim()}:${scope.workflowRunId.trim()}`;
  const existing = autoSyncTimers.get(key);
  if (existing) {
    clearTimeout(existing);
  }

  const uniqueArtifactIds = Array.from(
    new Set(scope.artifactIds.map((artifactId) => artifactId.trim()).filter(Boolean)),
  );
  if (uniqueArtifactIds.length === 0) {
    return;
  }

  autoSyncTimers.set(
    key,
    setTimeout(() => {
      autoSyncTimers.delete(key);
      void Promise.resolve().then(async () => {
        const localArtifacts = await dependencies.localRunnerGateway.listArtifacts();
        await syncEligibleArtifacts(
          dependencies,
          localArtifacts.filter((artifact) => uniqueArtifactIds.includes(artifact.artifactId)),
        );
      });
    }, delayMs),
  );
}

export async function bootstrapArtifactAutoSync(
  dependencies: ArtifactSyncDependencies,
) {
  if (autoSyncBootstrapRequested) {
    return;
  }
  autoSyncBootstrapRequested = true;

  const localArtifacts = await dependencies.localRunnerGateway.listArtifacts();
  await syncEligibleArtifacts(dependencies, localArtifacts);
}
