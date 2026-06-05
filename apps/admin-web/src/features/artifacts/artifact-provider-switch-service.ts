import type { SupabaseClient } from "@supabase/supabase-js";

import type { ArtifactStorageConnectionGateway } from "@/domain/gateway/artifact-storage-connection-gateway";
import type { ArtifactStorageProvider } from "@/domain/gateway/artifact-storage-gateway";
import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";
import type { ArtifactRunReplica } from "@/domain/model/entity/workflow-engine";
import { getArtifactStorageConnection } from "@/features/artifacts/artifact-storage-connection";
import { syncArtifactThroughRunner } from "@/features/artifacts/artifact-sync-service";

type ArtifactProviderSwitchDependencies = {
  supabase: SupabaseClient;
  localRunnerGateway: Pick<
    LocalRunnerGateway,
    | "getArtifactById"
    | "listArtifacts"
    | "exportArtifactSyncBundle"
    | "saveArtifactCloudSyncResult"
    | "hydrateArtifactFromRemote"
  >;
  artifactStorageConnectionGateway: ArtifactStorageConnectionGateway;
};

type ArtifactRunRow = {
  id: string;
  project_id: string | null;
  workflow_run_id: string | null;
  workflow_run_step_id: string | null;
  artifact_definition_key: string | null;
  title: string | null;
  created_at: string | null;
  updated_at: string | null;
};

export type ArtifactProviderSwitchResult = {
  status: "completed" | "failed";
  sourceProvider: ArtifactStorageProvider;
  targetProvider: ArtifactStorageProvider;
  totalArtifacts: number;
  syncedCount: number;
  skippedCount: number;
  failedCount: number;
  failures: Array<{
    artifactRunId: string;
    reason: string;
  }>;
};

const ARTIFACT_SUPABASE_BUCKET_NAME = "flowpilot-artifacts";

function normalizeProvider(value: string): ArtifactStorageProvider {
  return value === "google_drive" ? "google_drive" : "supabase";
}

function deriveSupabaseStorageScopeKey() {
  const baseUrl = process.env.SUPABASE_API_URL?.trim().replace(/\/+$/, "") ?? "";
  if (!baseUrl) {
    return null;
  }

  return `supabase:${baseUrl}:${ARTIFACT_SUPABASE_BUCKET_NAME}`;
}

function parseWorkflowStepKey(remotePath: string) {
  const normalized = remotePath.replaceAll("\\", "/").trim();
  const segments = normalized.split("/").filter(Boolean);
  const stepIndex = segments.findIndex((segment) => segment === "steps");
  if (stepIndex >= 0 && segments[stepIndex + 1]) {
    return segments[stepIndex + 1];
  }
  return "";
}

function selectRemoteSourceReplica(
  replicas: ArtifactRunReplica[],
  targetProvider: ArtifactStorageProvider,
  currentScopes: Partial<Record<ArtifactStorageProvider, string>>,
) {
  const prioritizedProviders: ArtifactStorageProvider[] = ["supabase", "google_drive"];
  for (const provider of prioritizedProviders) {
    if (provider === targetProvider) {
      continue;
    }
    const replica = replicas.find(
      (candidate) =>
        candidate.provider === provider &&
        candidate.syncStatus === "synced" &&
        candidate.remotePath.trim() !== "" &&
        (!currentScopes[provider] ||
          !candidate.storageScopeKey ||
          candidate.storageScopeKey === currentScopes[provider]),
    );
    if (replica) {
      return replica;
    }
  }
  return null;
}

function hasOutOfScopeRemoteReplica(
  replicas: ArtifactRunReplica[],
  targetProvider: ArtifactStorageProvider,
  currentScopes: Partial<Record<ArtifactStorageProvider, string>>,
) {
  return replicas.some((replica) => {
    const currentScopeKey = currentScopes[replica.provider];
    return (
      replica.provider !== targetProvider &&
      replica.syncStatus === "synced" &&
      replica.remotePath.trim() !== "" &&
      Boolean(currentScopeKey) &&
      Boolean(replica.storageScopeKey) &&
      replica.storageScopeKey !== currentScopeKey
    );
  });
}

async function resolveCurrentStorageScopes(
  dependencies: ArtifactProviderSwitchDependencies,
  projectId: string,
) {
  const scopes: Partial<Record<ArtifactStorageProvider, string>> = {};
  const supabaseScopeKey = deriveSupabaseStorageScopeKey();
  if (supabaseScopeKey) {
    scopes.supabase = supabaseScopeKey;
  }

  const connection = await getArtifactStorageConnection(
    dependencies.supabase,
    projectId,
    "google_drive",
  );
  if (connection?.folderId.trim()) {
    scopes.google_drive = `gdrive:${connection.folderId.trim()}`;
  }

  return scopes;
}

async function ensureTargetProviderReady(
  dependencies: ArtifactProviderSwitchDependencies,
  projectId: string,
  targetProvider: ArtifactStorageProvider,
) {
  if (targetProvider !== "google_drive") {
    return;
  }

  const connection = await getArtifactStorageConnection(
    dependencies.supabase,
    projectId,
    "google_drive",
  );
  if (!connection) {
    throw new Error("Google Drive artifact storage is not connected on this runner for the selected project.");
  }
  if (connection.status === "failed" || connection.status === "reconnect_required") {
    throw new Error("Google Drive artifact storage requires reconnect on this runner before switching providers.");
  }
  if (connection.status !== "connected" || !connection.folderId.trim()) {
    throw new Error("Connect a Google Drive folder on this runner before switching providers.");
  }
}

export async function switchProjectArtifactStorageProvider(
  projectId: string,
  targetProviderInput: string,
  dependencies: ArtifactProviderSwitchDependencies,
) : Promise<ArtifactProviderSwitchResult> {
  const trimmedProjectId = projectId.trim();
  if (!trimmedProjectId) {
    throw new Error("projectId is required.");
  }

  const targetProvider = normalizeProvider(targetProviderInput.trim());
  const sourceProvider =
    await dependencies.artifactStorageConnectionGateway.getProjectStorageProvider(trimmedProjectId);
  const currentScopes = await resolveCurrentStorageScopes(dependencies, trimmedProjectId);

  await ensureTargetProviderReady(dependencies, trimmedProjectId, targetProvider);

  if (sourceProvider === targetProvider) {
    return {
      status: "completed",
      sourceProvider,
      targetProvider,
      totalArtifacts: 0,
      syncedCount: 0,
      skippedCount: 0,
      failedCount: 0,
      failures: [],
    };
  }

  const [{ data, error }, localArtifacts] = await Promise.all([
    dependencies.supabase
      .from("artifact_runs")
      .select("id, project_id, workflow_run_id, workflow_run_step_id, artifact_definition_key, title, created_at, updated_at")
      .eq("project_id", trimmedProjectId),
    dependencies.localRunnerGateway.listArtifacts(),
  ]);

  if (error) {
    throw new Error(`Unable to load project artifacts for provider switch: ${error.message}`);
  }

  const artifactRuns = (data ?? []) as ArtifactRunRow[];
  const artifactRunById = new Map(
    artifactRuns.map((artifactRun) => [artifactRun.id, artifactRun] as const),
  );
  const replicasByArtifactRunId =
    await dependencies.artifactStorageConnectionGateway.listArtifactRunReplicas(
      artifactRuns.map((artifactRun) => artifactRun.id),
    );
  const localArtifactsById = new Map(
    localArtifacts
      .filter((artifact) => artifact.projectId.trim() === trimmedProjectId)
      .map((artifact) => [artifact.artifactId, artifact] as const),
  );

  const candidateArtifactIds = new Set<string>();
  for (const artifactRun of artifactRuns) {
    candidateArtifactIds.add(artifactRun.id);
  }
  for (const artifactId of localArtifactsById.keys()) {
    candidateArtifactIds.add(artifactId);
  }

  const failures: ArtifactProviderSwitchResult["failures"] = [];
  let syncedCount = 0;
  let skippedCount = 0;

  const orderedArtifactIds = Array.from(candidateArtifactIds).sort();
  for (const artifactId of orderedArtifactIds) {
    const localArtifact = localArtifactsById.get(artifactId) ?? null;
    const artifactRun = artifactRunById.get(artifactId) ?? null;
    const replicas = replicasByArtifactRunId[artifactId] ?? [];
    const targetReplica =
      replicas.find(
        (replica) =>
          replica.provider === targetProvider &&
          replica.syncStatus === "synced" &&
          replica.remotePath.trim() !== "",
      ) ?? null;

    if (targetReplica && (!localArtifact?.checksum || targetReplica.checksum === localArtifact.checksum)) {
      skippedCount += 1;
      continue;
    }

    if (!localArtifact) {
      const sourceReplica = selectRemoteSourceReplica(replicas, targetProvider, currentScopes);
      if (artifactRun && sourceReplica) {
        const workflowStepKey =
          parseWorkflowStepKey(sourceReplica.remotePath) ||
          parseWorkflowStepKey(targetReplica?.remotePath ?? "") ||
          "unknown_step";
        try {
          const hydratedArtifact = await dependencies.localRunnerGateway.hydrateArtifactFromRemote(artifactId, {
            artifactId,
            title: artifactRun.title?.trim() || "Artifact",
            sourceKind: artifactRun.artifact_definition_key?.trim() || "workflow_output",
            projectId: trimmedProjectId,
            featureId: workflowStepKey,
            workflowRunId: artifactRun.workflow_run_id?.trim() || "",
            workflowStepKey,
            providerKey: "codex",
            remotePath: sourceReplica.remotePath,
            remoteObjectId: sourceReplica.remoteObjectId ?? undefined,
            sourceStorageProvider: sourceReplica.provider,
            createdAt: artifactRun.created_at?.trim() || undefined,
            updatedAt: artifactRun.updated_at?.trim() || undefined,
          });
          localArtifactsById.set(artifactId, hydratedArtifact);
        } catch (error) {
          failures.push({
            artifactRunId: artifactId,
            reason: error instanceof Error ? error.message : "Unable to hydrate artifact from remote source.",
          });
          continue;
        }
      }
    }

    const hydratedLocalArtifact =
      localArtifactsById.get(artifactId) ??
      (await dependencies.localRunnerGateway.getArtifactById(artifactId));
    if (!hydratedLocalArtifact) {
      failures.push({
        artifactRunId: artifactId,
        reason: hasOutOfScopeRemoteReplica(replicas, targetProvider, currentScopes)
          ? "source replica exists outside the current runner storage scope"
          : "source_unavailable_on_this_runner",
      });
      continue;
    }

    try {
      await syncArtifactThroughRunner(artifactId, dependencies, { targetProvider });
      localArtifactsById.set(artifactId, hydratedLocalArtifact);
      syncedCount += 1;
    } catch (error) {
      failures.push({
        artifactRunId: artifactId,
        reason: error instanceof Error ? error.message : "Unable to sync artifact.",
      });
    }
  }

  if (failures.length === 0) {
    const { error: updateError } = await dependencies.supabase
      .from("projects")
      .update({
        artifact_storage_preference: targetProvider,
      })
      .eq("id", trimmedProjectId);
    if (updateError) {
      throw new Error(`Unable to update project artifact storage provider: ${updateError.message}`);
    }
  }

  return {
    status: failures.length === 0 ? "completed" : "failed",
    sourceProvider,
    targetProvider,
    totalArtifacts: orderedArtifactIds.length,
    syncedCount,
    skippedCount,
    failedCount: failures.length,
    failures,
  };
}
