import type { LocalRunnerGateway } from "@/domain/gateway/local-runner-gateway";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";

const workflowRunPathPattern =
  /\.flowpilot[\\/]+artifacts[\\/]+[^/\\\s]+[\\/]+([0-9a-f-]{36})[\\/]+/gi;

function normalize(value: string | null | undefined) {
  return (value ?? "").trim().toLowerCase();
}

export function summarizeRunPrompt(promptText?: string) {
  const normalized = promptText?.trim() ?? "";
  if (!normalized) {
    return null;
  }

  const beginPromptMarker = "## Begin Prompt";
  const beginPromptIndex = normalized.indexOf(beginPromptMarker);

  if (beginPromptIndex >= 0) {
    const afterMarker = normalized
      .slice(beginPromptIndex + beginPromptMarker.length)
      .trim();
    const nextSectionMatch = afterMarker.match(/^##\s+/m);
    const nextSectionIndex = nextSectionMatch?.index ?? -1;
    const beginPromptSection =
      nextSectionIndex >= 0
        ? afterMarker.slice(0, nextSectionIndex).trim()
        : afterMarker;
    const singleLinePrompt = beginPromptSection.replace(/\s+/g, " ").trim();

    if (singleLinePrompt) {
      return singleLinePrompt.length > 88
        ? `${singleLinePrompt.slice(0, 85).trimEnd()}...`
        : singleLinePrompt;
    }
  }

  const firstLine = normalized
    .split("\n")
    .map((line) => line.trim())
    .find(Boolean);

  if (!firstLine) {
    return null;
  }

  return firstLine.length > 88
    ? `${firstLine.slice(0, 85).trimEnd()}...`
    : firstLine;
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

function selectLatestArtifactByRun(
  artifacts: LocalRunnerArtifact[],
  runIds: Set<string>,
) {
  const latestByRun = new Map<string, LocalRunnerArtifact>();

  for (const artifact of artifacts) {
    if (!isWorkflowOutputArtifact(artifact)) {
      continue;
    }

    const normalizedRunId = normalize(artifact.workflowRunId);
    if (!runIds.has(normalizedRunId)) {
      continue;
    }

    const current = latestByRun.get(normalizedRunId);
    if (!current || artifact.updatedAt > current.updatedAt) {
      latestByRun.set(normalizedRunId, artifact);
    }
  }

  return latestByRun;
}

function sortByUpdatedAtDesc(left: LocalRunnerArtifact, right: LocalRunnerArtifact) {
  return right.updatedAt.localeCompare(left.updatedAt);
}

export async function loadWorkflowRunTitleMap(
  localRunnerGateway: Pick<
    LocalRunnerGateway,
    "listArtifacts" | "getArtifactById"
  >,
  runIds: string[],
) {
  const normalizedRunIds = new Set(
    runIds.map((runId) => normalize(runId)).filter(Boolean),
  );
  if (normalizedRunIds.size === 0) {
    return new Map<string, string>();
  }

  try {
    const artifacts = await localRunnerGateway.listArtifacts();
    const latestByRun = selectLatestArtifactByRun(artifacts, normalizedRunIds);
    const resolvedTitles = new Map<string, string>();
    const entries = await Promise.all(
      Array.from(latestByRun.entries()).map(async ([runId, artifact]) => {
        const artifactDetail =
          (await localRunnerGateway.getArtifactById(artifact.artifactId)) ??
          artifact;
        const title = summarizeRunPrompt(artifactDetail.promptText);
        return title ? ([runId, title] as const) : null;
      }),
    );

    for (const entry of entries) {
      if (!entry) {
        continue;
      }
      resolvedTitles.set(entry[0], entry[1]);
    }

    const unresolvedRunIds = new Set(
      Array.from(normalizedRunIds).filter((runId) => !resolvedTitles.has(runId)),
    );
    if (unresolvedRunIds.size === 0) {
      return resolvedTitles;
    }

    const promptExecutionArtifacts = artifacts
      .filter(isPromptExecutionArtifact)
      .sort(sortByUpdatedAtDesc);

    for (const artifact of promptExecutionArtifacts) {
      if (unresolvedRunIds.size === 0) {
        break;
      }

      const artifactDetail =
        (await localRunnerGateway.getArtifactById(artifact.artifactId)) ??
        artifact;
      const resolvedRunId = extractWorkflowRunIdFromPrompt(
        artifactDetail.promptText,
      );
      if (!resolvedRunId || !unresolvedRunIds.has(resolvedRunId)) {
        continue;
      }

      const title = summarizeRunPrompt(artifactDetail.promptText);
      if (!title) {
        continue;
      }

      resolvedTitles.set(resolvedRunId, title);
      unresolvedRunIds.delete(resolvedRunId);
    }

    return resolvedTitles;
  } catch {
    return new Map<string, string>();
  }
}
