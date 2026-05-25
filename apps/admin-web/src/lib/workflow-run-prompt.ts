import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";

const workflowRunPathPattern =
  /\.flowpilot\/artifacts\/[^/\s]+\/([0-9a-f-]{36})\//gi;

function normalize(value: string | null | undefined) {
  return (value ?? "").trim().toLowerCase();
}

function isWorkflowOutputArtifact(artifact: LocalRunnerArtifact) {
  return (
    normalize(artifact.sourceKind) === "workflow_output" &&
    normalize(artifact.workflowRunId) !== ""
  );
}

function isPromptExecutionArtifact(artifact: LocalRunnerArtifact) {
  return normalize(artifact.sourceKind) === "prompt_execution";
}

function extractWorkflowRunIdFromPrompt(promptText?: string) {
  const normalizedPrompt = promptText?.trim();
  if (!normalizedPrompt) {
    return null;
  }

  const matches = normalizedPrompt.matchAll(workflowRunPathPattern);
  for (const match of matches) {
    const runId = normalize(match[1]);
    if (runId) {
      return runId;
    }
  }

  return null;
}

function sortByUpdatedAtDesc(left: LocalRunnerArtifact, right: LocalRunnerArtifact) {
  return right.updatedAt.localeCompare(left.updatedAt);
}

export async function loadWorkflowRunPromptText(
  localRunnerGateway: Pick<
    LocalRunnerGateway,
    "listArtifacts" | "getArtifactById"
  >,
  runId: string,
) {
  const normalizedRunId = normalize(runId);
  if (!normalizedRunId) {
    return null;
  }

  try {
    const artifacts = await localRunnerGateway.listArtifacts();

    const workflowOutputArtifacts = artifacts
      .filter(
        (artifact) =>
          isWorkflowOutputArtifact(artifact) &&
          normalize(artifact.workflowRunId) === normalizedRunId,
      )
      .sort(sortByUpdatedAtDesc);

    for (const artifact of workflowOutputArtifacts) {
      const artifactDetail =
        (await localRunnerGateway.getArtifactById(artifact.artifactId)) ??
        artifact;
      const promptText = artifactDetail.promptText?.trim();
      if (promptText) {
        return promptText;
      }
    }

    const promptExecutionArtifacts = artifacts
      .filter(isPromptExecutionArtifact)
      .sort(sortByUpdatedAtDesc);

    for (const artifact of promptExecutionArtifacts) {
      const artifactDetail =
        (await localRunnerGateway.getArtifactById(artifact.artifactId)) ??
        artifact;
      const resolvedRunId = extractWorkflowRunIdFromPrompt(
        artifactDetail.promptText,
      );
      if (resolvedRunId !== normalizedRunId) {
        continue;
      }

      const promptText = artifactDetail.promptText?.trim();
      if (promptText) {
        return promptText;
      }
    }

    return null;
  } catch {
    return null;
  }
}
