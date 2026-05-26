import type { ApprovalDecision } from "@/domain/model/entity/workflow";
import type { WorkflowOutputRecord } from "@/features/workflow-engine/workflow-run-detail-timeline";

const BEGIN_PROMPT_PREFIX = "Begin prompt:";
const AI_OUTPUT_PREFIX = "ai_output:";
const APPROVAL_DECISION_PREFIX = "approval_decision:";

type WorkflowRunLogLike = {
  workflowRunStepId: string;
  logLevel: string;
  message: string;
  createdAt: string;
};

type WorkflowStepLike = {
  id: string;
  stepName?: string;
};

function normalize(value: string | null | undefined) {
  return (value ?? "").trim();
}

function extractAiOutputPayload(message: string) {
  const normalizedMessage = normalize(message);
  if (!normalizedMessage.startsWith(AI_OUTPUT_PREFIX)) {
    return null;
  }

  return normalizedMessage.slice(AI_OUTPUT_PREFIX.length).trimStart();
}

export function extractBeginPromptFromLogs(logs: WorkflowRunLogLike[]) {
  for (const log of logs) {
    const message = normalize(log.message);
    if (!message.startsWith(BEGIN_PROMPT_PREFIX)) {
      continue;
    }

    const prompt = message.slice(BEGIN_PROMPT_PREFIX.length).trim();
    if (prompt) {
      return prompt;
    }
  }

  return null;
}

function extractApprovalDecisionPayload(message: string) {
  const normalizedMessage = normalize(message);
  if (!normalizedMessage.startsWith(APPROVAL_DECISION_PREFIX)) {
    return null;
  }

  return normalizedMessage.slice(APPROVAL_DECISION_PREFIX.length).trimStart();
}

export function buildFallbackApprovalDecisionsFromLogs(
  logs: WorkflowRunLogLike[],
): ApprovalDecision[] {
  const merged = new Map<string, ApprovalDecision>();

  for (const log of logs) {
    const payload = extractApprovalDecisionPayload(log.message);
    if (!payload) {
      continue;
    }

    try {
      const parsed = JSON.parse(payload) as Partial<ApprovalDecision>;
      const id = normalize(parsed.id);
      const workflowRunId = normalize(parsed.workflowRunId);
      const workflowStepId = normalize(parsed.workflowStepId);
      const decision = normalize(parsed.decision) as ApprovalDecision["decision"];
      const createdAt = normalize(parsed.createdAt);

      if (!id || !workflowRunId || !workflowStepId || !decision || !createdAt) {
        continue;
      }

      merged.set(id, {
        id,
        approvalId: normalize(parsed.approvalId) || `approval_${workflowStepId}`,
        workflowRunId,
        workflowStepId,
        aiOutputId: parsed.aiOutputId ?? null,
        decision,
        reviewerId: parsed.reviewerId ?? null,
        comment: parsed.comment ?? null,
        createdAt,
      });
    } catch {
      continue;
    }
  }

  return Array.from(merged.values()).sort((left, right) =>
    left.createdAt.localeCompare(right.createdAt),
  );
}

function extractRenderableStepOutput(log: WorkflowRunLogLike) {
  const message = normalize(log.message);
  if (!message || message.startsWith("session_event:")) {
    return null;
  }

  const aiOutputPayload = extractAiOutputPayload(message);
  if (aiOutputPayload) {
    return aiOutputPayload;
  }

  if (normalize(log.logLevel).toLowerCase() !== "debug") {
    return null;
  }

  return message;
}

export function buildFallbackOutputsFromLogs({
  existingOutputs,
  logs,
  projectId,
  runId,
  steps,
}: {
  existingOutputs: WorkflowOutputRecord[];
  logs: WorkflowRunLogLike[];
  projectId: string;
  runId: string;
  steps: WorkflowStepLike[];
}) {
  const latestLogByStepId = new Map<string, WorkflowRunLogLike>();
  const latestOutputIdByStepId = new Map<string, string>();

  for (const log of logs) {
    const stepId = normalize(log.workflowRunStepId);
    if (!stepId) {
      continue;
    }
    if (!extractRenderableStepOutput(log)) {
      continue;
    }

    const current = latestLogByStepId.get(stepId);
    if (!current || log.createdAt > current.createdAt) {
      latestLogByStepId.set(stepId, log);
    }
  }

  for (const output of existingOutputs) {
    const currentOutputId = latestOutputIdByStepId.get(output.workflowStepId);
    if (!currentOutputId) {
      latestOutputIdByStepId.set(output.workflowStepId, output.id);
      continue;
    }

    const currentOutput = existingOutputs.find((candidate) => candidate.id === currentOutputId);
    if (!currentOutput || output.createdAt > currentOutput.createdAt) {
      latestOutputIdByStepId.set(output.workflowStepId, output.id);
    }
  }

  const enrichedOutputs = existingOutputs.map((output) => {
    if (latestOutputIdByStepId.get(output.workflowStepId) !== output.id) {
      return output;
    }

    const log = latestLogByStepId.get(output.workflowStepId);
    const content = log ? extractRenderableStepOutput(log) : null;
    if (!content) {
      return output;
    }

    return {
      ...output,
      contentMarkdown: content,
      stdoutText: output.stdoutText || content,
    } satisfies WorkflowOutputRecord;
  });

  const stepIdsWithOutputs = new Set(enrichedOutputs.map((output) => output.workflowStepId));
  const synthesizedOutputs = steps.flatMap((step) => {
    if (stepIdsWithOutputs.has(step.id)) {
      return [];
    }

    const log = latestLogByStepId.get(step.id);
    const content = log ? extractRenderableStepOutput(log) : null;
    if (!content) {
      return [];
    }

    return [
      {
        id: `log-output-${step.id}`,
        workflowRunId: runId,
        workflowStepId: step.id,
        projectId,
        outputType: "document",
        version: 1,
        title: `${normalize(step.stepName) || "Runner Output"}.md`,
        contentMarkdown: content,
        isApproved: true,
        createdAt: log.createdAt,
        stdoutText: content,
      } satisfies WorkflowOutputRecord,
    ];
  });

  return [...enrichedOutputs, ...synthesizedOutputs];
}
