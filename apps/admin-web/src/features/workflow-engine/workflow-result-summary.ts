import type { WorkflowStep } from "@/domain/model/entity/workflow";
import type { WorkflowRunStep } from "@/domain/model/entity/workflow-engine";

export const RESULT_SUMMARY_STEP_TYPE = "result_summary";
export const RESULT_SUMMARY_STEP_NAME = "Summary";
const RESULT_SUMMARY_MIN_MAIN_STEPS = 2;
const RESULT_SUMMARY_OUTPUT_LIMIT = 1600;

export type ResultSummarySourceStep = {
  stepName: string;
  stepType: string;
  outputMarkdown: string;
  artifactOutputPaths: string[];
  startedAt: string;
  completedAt: string;
};

function normalize(value: string | null | undefined) {
  return (value ?? "").trim().toLowerCase();
}

function trimForPrompt(value: string, maxLength = RESULT_SUMMARY_OUTPUT_LIMIT) {
  const trimmed = value.trim();
  if (trimmed.length <= maxLength) {
    return trimmed;
  }

  return `${trimmed.slice(0, maxLength)}\n...[truncated]`;
}

function mapEngineStepStatus(status: string): WorkflowStep["status"] {
  const normalized = normalize(status);
  if (normalized === "pending") return "pending";
  if (normalized === "running") return "running";
  if (normalized === "waiting_user_approval") return "waiting_approval";
  if (normalized === "done" || normalized === "completed") return "completed";
  if (normalized === "failed") return "failed";
  if (normalized === "skipped" || normalized === "rejected") return "rejected";
  return normalized as WorkflowStep["status"];
}

export function shouldAppendResultSummaryStep(mainSteps: { step_type?: string | null }[]) {
  if (mainSteps.length < RESULT_SUMMARY_MIN_MAIN_STEPS) {
    return false;
  }
  const lastStep = mainSteps[mainSteps.length - 1];
  if (isResultSummaryStepType(lastStep?.step_type)) {
    return false;
  }
  return true;
}

export function isResultSummaryStepType(stepType: string | null | undefined) {
  return normalize(stepType) === RESULT_SUMMARY_STEP_TYPE;
}

export function buildResultSummaryPrompt({
  beginPrompt,
  workflowName,
  workingDirectory,
  steps,
}: {
  beginPrompt: string;
  workflowName: string;
  workingDirectory: string;
  steps: ResultSummarySourceStep[];
}) {
  const renderedSteps = steps.map((step, index) => {
    const artifactPaths =
      step.artifactOutputPaths.length > 0
        ? step.artifactOutputPaths.map((artifactPath) => `- ${artifactPath}`).join("\n")
        : "- None";

    return [
      `### Step ${index + 1}: ${step.stepName}`,
      `- Step type: ${step.stepType}`,
      `- Started at: ${step.startedAt}`,
      `- Completed at: ${step.completedAt}`,
      "#### Output Excerpt",
      trimForPrompt(step.outputMarkdown),
      "#### Artifact Output Paths",
      artifactPaths,
    ].join("\n");
  });

  return [
    "Create a final workflow-level summary based on the completed steps below.",
    "Return concise Markdown that explains the overall result of the workflow.",
    "Include: overall objective, completed work, key decisions, final deliverables, and remaining risks or follow-up items.",
    "Do not summarize this summary step itself. Summarize only the earlier completed steps.",
    "",
    `## Workflow`,
    `- Name: ${workflowName}`,
    `- Working directory: ${workingDirectory}`,
    "",
    "## Original Goal",
    beginPrompt.trim(),
    "",
    "## Completed Step Results",
    renderedSteps.join("\n\n"),
  ].join("\n");
}

export function buildSyntheticResultSummaryStep({
  runId,
  step,
}: {
  runId: string;
  step: WorkflowRunStep;
}): WorkflowStep {
  return {
    id: step.id,
    workflowRunId: runId,
    stepKey: RESULT_SUMMARY_STEP_TYPE,
    stepName: RESULT_SUMMARY_STEP_NAME,
    stepType: "ai_mock",
    status: mapEngineStepStatus(step.status),
    sequenceIndex: step.executionOrderIndex,
    outputId: step.artifactRunId ?? step.artifactId ?? null,
    startedAt: step.startedAt,
    completedAt: step.finishedAt,
    errorMessage: step.errorMessage,
  };
}

export function appendSyntheticResultSummarySteps({
  baseSteps,
  engineSteps,
  runId,
}: {
  baseSteps: WorkflowStep[];
  engineSteps: WorkflowRunStep[];
  runId: string;
}) {
  const syntheticSteps = engineSteps
    .filter(
      (step) =>
        step.workflowStepId == null &&
        isResultSummaryStepType(step.stepType) &&
        !baseSteps.some((candidate) => candidate.id === step.id),
    )
    .map((step) => buildSyntheticResultSummaryStep({ runId, step }));

  return [...baseSteps, ...syntheticSteps].sort(
    (left, right) => left.sequenceIndex - right.sequenceIndex,
  );
}
