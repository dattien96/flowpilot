import type { ArtifactRun } from "@/domain/model/entity/workflow-engine";
import type { WorkflowOutputRecord } from "@/features/workflow-engine/workflow-run-detail-timeline";

function normalize(value: string | null | undefined) {
  return (value ?? "").trim().toLowerCase();
}

export function resolveArtifactRunForOutput(
  output: WorkflowOutputRecord,
  artifactRuns: ArtifactRun[],
) {
  const exactMatch = artifactRuns.find((artifactRun) => artifactRun.id === output.id);
  if (exactMatch) {
    return exactMatch;
  }

  const outputStepId = normalize(output.workflowStepId);
  if (!outputStepId) {
    return null;
  }

  const stepMatches = artifactRuns.filter(
    (artifactRun) => normalize(artifactRun.workflowRunStepId) === outputStepId,
  );
  if (stepMatches.length === 0) {
    return null;
  }

  const outputTitle = normalize(output.title);
  const titleMatches = outputTitle
    ? stepMatches.filter((artifactRun) => normalize(artifactRun.title) === outputTitle)
    : [];
  const candidates = titleMatches.length > 0 ? titleMatches : stepMatches;

  const outputCreatedAt = Date.parse(output.createdAt);
  if (Number.isNaN(outputCreatedAt)) {
    return candidates[0] ?? null;
  }

  return (
    [...candidates].sort((left, right) => {
      const leftDistance = Math.abs(Date.parse(left.createdAt) - outputCreatedAt);
      const rightDistance = Math.abs(Date.parse(right.createdAt) - outputCreatedAt);
      return leftDistance - rightDistance;
    })[0] ?? null
  );
}
