import type { ApprovalDecision, WorkflowStep } from "@/domain/model/entity/workflow";
import type { WorkflowRunDetail } from "@/domain/model/response/workflow-response";

function buildOptimisticDecision(
  detail: WorkflowRunDetail,
  stepId: string,
  comment: string,
  createdAt: string,
): ApprovalDecision {
  return {
    id: `optimistic-follow-up-${stepId}-${createdAt}`,
    approvalId: `approval_${stepId}`,
    workflowRunId: detail.run.id,
    workflowStepId: stepId,
    aiOutputId: null,
    decision: "changes_requested",
    reviewerId: null,
    comment,
    createdAt,
  };
}

function updateStep(step: WorkflowStep, stepId: string) {
  if (step.id !== stepId) {
    return step;
  }

  return {
    ...step,
    status: "running",
    completedAt: null,
    errorMessage: null,
  } satisfies WorkflowStep;
}

export function applyOptimisticWorkflowFollowUp(
  detail: WorkflowRunDetail,
  stepId: string,
  comment: string,
  createdAt: string,
): WorkflowRunDetail {
  const targetStep = detail.steps.find((step) => step.id === stepId) ?? null;
  const optimisticDecision = buildOptimisticDecision(
    detail,
    stepId,
    comment,
    createdAt,
  );

  return {
    ...detail,
    run: {
      ...detail.run,
      status: "running",
      currentStepKey: targetStep?.stepKey ?? detail.run.currentStepKey,
      completedAt: null,
      errorSummary: null,
    },
    steps: detail.steps.map((step) => updateStep(step, stepId)),
    approvals: detail.approvals.filter(
      (approval) =>
        approval.workflowStepId !== stepId || approval.status !== "pending",
    ),
    approvalDecisions: [
      ...(detail.approvalDecisions ?? []),
      optimisticDecision,
    ],
  };
}
