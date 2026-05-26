import type { ApprovalDecision, WorkflowStep } from "@/domain/model/entity/workflow";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";

export interface WorkflowOutputRecord {
  id: string;
  workflowRunId: string;
  workflowStepId: string;
  projectId: string;
  outputType: string;
  version: number;
  title: string;
  contentMarkdown: string;
  isApproved: boolean;
  createdAt: string;
  promptText?: string;
  stdoutText?: string;
  stderrText?: string;
  commandText?: string;
  localPath?: string;
}

export type WorkflowStepTimelineItem =
  | {
      kind: "output";
      key: string;
      createdAt: string;
      output: WorkflowOutputRecord;
    }
  | {
      kind: "decision";
      key: string;
      createdAt: string;
      decision: ApprovalDecision;
    };

function normalize(value: string | null | undefined) {
  return (value ?? "").trim().toLowerCase();
}

function compareCreatedAtAsc(left: { createdAt: string }, right: { createdAt: string }) {
  return left.createdAt.localeCompare(right.createdAt);
}

function findStepForArtifact(steps: WorkflowStep[], artifact: LocalRunnerArtifact) {
  const artifactStepKey = normalize(artifact.workflowStepKey);
  return (
    steps.find((step) => normalize(step.stepKey) === artifactStepKey) ??
    steps.find((step) => normalize(step.stepName) === artifactStepKey) ??
    null
  );
}

export function mapArtifactsToWorkflowOutputs(
  artifacts: LocalRunnerArtifact[],
  steps: WorkflowStep[],
): WorkflowOutputRecord[] {
  return artifacts.map((artifact) => {
    const step = findStepForArtifact(steps, artifact);

    return {
      id: artifact.artifactId,
      workflowRunId: artifact.workflowRunId,
      workflowStepId: step ? step.id : artifact.workflowStepKey,
      projectId: artifact.projectId,
      outputType: "document",
      version: 1,
      title: artifact.title,
      contentMarkdown: artifact.contentMarkdown,
      isApproved: true,
      createdAt: artifact.createdAt,
      promptText: artifact.promptText,
      stdoutText: artifact.stdoutText,
      stderrText: artifact.stderrText,
      commandText: artifact.commandText,
      localPath: artifact.localPath,
    };
  });
}

export function mergeWorkflowOutputs(
  baseOutputs: WorkflowOutputRecord[],
  localOutputs: WorkflowOutputRecord[],
) {
  const merged = new Map<string, WorkflowOutputRecord>();

  for (const output of baseOutputs) {
    merged.set(output.id, output);
  }

  for (const output of localOutputs) {
    const current = merged.get(output.id);
    merged.set(output.id, current ? { ...current, ...output } : output);
  }

  return Array.from(merged.values()).sort(compareCreatedAtAsc);
}

export function groupOutputsByStep(
  outputs: WorkflowOutputRecord[],
) {
  const grouped = new Map<string, WorkflowOutputRecord[]>();

  for (const output of outputs) {
    const current = grouped.get(output.workflowStepId) ?? [];
    current.push(output);
    current.sort(compareCreatedAtAsc);
    grouped.set(output.workflowStepId, current);
  }

  return grouped;
}

export function buildWorkflowStepTimeline(
  outputs: WorkflowOutputRecord[],
  decisions: ApprovalDecision[],
): WorkflowStepTimelineItem[] {
  const items: WorkflowStepTimelineItem[] = [
    ...outputs.map((output) => ({
      kind: "output" as const,
      key: `output-${output.id}`,
      createdAt: output.createdAt,
      output,
    })),
    ...decisions.map((decision) => ({
      kind: "decision" as const,
      key: `decision-${decision.id}`,
      createdAt: decision.createdAt,
      decision,
    })),
  ];

  return items.sort((left, right) => {
    const createdAtCompare = left.createdAt.localeCompare(right.createdAt);
    if (createdAtCompare !== 0) {
      return createdAtCompare;
    }

    if (left.kind === right.kind) {
      return left.key.localeCompare(right.key);
    }

    return left.kind === "decision" ? -1 : 1;
  });
}
