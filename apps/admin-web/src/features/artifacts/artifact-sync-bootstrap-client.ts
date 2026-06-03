import type { LocalRunnerHealth } from "@/domain/model/entity/local-runner";

const ARTIFACT_SYNC_BOOTSTRAP_PATH = "/api/local-runner/artifacts/bootstrap-sync";

let completedRunnerStartedAt = "";
let pendingRunnerStartedAt = "";
let pendingBootstrapRequest: Promise<boolean> | null = null;

function resolveRunnerStartedAt(health: LocalRunnerHealth) {
  return health.startedAt?.trim() ?? "";
}

export async function requestArtifactSyncBootstrap(
  health: LocalRunnerHealth,
  fetchImpl: typeof fetch = fetch,
) {
  if (health.status !== "online") {
    return false;
  }

  const runnerStartedAt = resolveRunnerStartedAt(health);
  if (runnerStartedAt === completedRunnerStartedAt) {
    return false;
  }

  if (pendingBootstrapRequest && runnerStartedAt === pendingRunnerStartedAt) {
    return pendingBootstrapRequest;
  }

  const request = fetchImpl(ARTIFACT_SYNC_BOOTSTRAP_PATH, {
    method: "POST",
    cache: "no-store",
  }).then(async (response) => {
    if (!response.ok) {
      const payload = (await response.json().catch(() => null)) as
        | { error?: string }
        | null;
      throw new Error(payload?.error ?? "Unable to start artifact sync bootstrap.");
    }

    completedRunnerStartedAt = runnerStartedAt;
    return true;
  });

  let trackedRequest: Promise<boolean>;
  pendingRunnerStartedAt = runnerStartedAt;
  trackedRequest = request.finally(() => {
    if (pendingBootstrapRequest === trackedRequest) {
      pendingBootstrapRequest = null;
      pendingRunnerStartedAt = "";
    }
  });
  pendingBootstrapRequest = trackedRequest;

  return pendingBootstrapRequest;
}

export function resetArtifactSyncBootstrapClientState() {
  completedRunnerStartedAt = "";
  pendingRunnerStartedAt = "";
  pendingBootstrapRequest = null;
}
