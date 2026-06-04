import type { ArtifactRun } from "@/domain/model/entity/workflow-engine";

export type WorkflowRunArtifactFile = "content" | "actual-prompt" | "prompt";

type ArtifactRunOpenTarget = Pick<
  ArtifactRun,
  "id" | "projectId" | "remoteObjectId" | "remotePath" | "storageProvider"
>;

const artifactTextRequestInit: RequestInit = {
  cache: "no-store",
  headers: {
    accept: "text/markdown",
  },
};

export function buildWorkflowRunArtifactOpenHref(
  artifactRun: ArtifactRunOpenTarget,
  file: WorkflowRunArtifactFile = "content",
) {
  const params = new URLSearchParams();
  const remotePath = artifactRun.remotePath.trim();
  if (remotePath) {
    params.set("remotePath", remotePath);
    params.set("storageProvider", artifactRun.storageProvider ?? "supabase");
  }
  if (artifactRun.remoteObjectId?.trim()) {
    params.set("remoteObjectId", artifactRun.remoteObjectId.trim());
  }
  if (artifactRun.projectId?.trim()) {
    params.set("projectId", artifactRun.projectId.trim());
  }
  if (file !== "content") {
    params.set("file", file);
  }

  const artifactId = encodeURIComponent(artifactRun.id.trim() || "remote-artifact");
  const query = params.toString();
  return query
    ? `/api/local-runner/artifacts/${artifactId}/open?${query}`
    : `/api/local-runner/artifacts/${artifactId}/open`;
}

export async function loadWorkflowRunArtifactContent(
  artifactRun: ArtifactRunOpenTarget,
  fetchImpl: typeof fetch = fetch,
) {
  return loadWorkflowRunArtifactFileText(artifactRun, "content", fetchImpl);
}

export async function loadWorkflowRunArtifactPromptFiles(
  artifactRun: ArtifactRunOpenTarget,
  fetchImpl: typeof fetch = fetch,
) {
  const [actualPromptText, promptText] = await Promise.all([
    loadWorkflowRunArtifactFileText(artifactRun, "actual-prompt", fetchImpl),
    loadWorkflowRunArtifactFileText(artifactRun, "prompt", fetchImpl),
  ]);

  return {
    actualPromptText,
    promptText,
  };
}

async function loadWorkflowRunArtifactFileText(
  artifactRun: ArtifactRunOpenTarget,
  file: WorkflowRunArtifactFile,
  fetchImpl: typeof fetch,
) {
  try {
    const response = await fetchImpl(
      buildWorkflowRunArtifactOpenHref(artifactRun, file),
      artifactTextRequestInit,
    );
    if (!response.ok) {
      return null;
    }

    return await response.text();
  } catch {
    return null;
  }
}
