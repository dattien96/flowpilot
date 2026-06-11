export type RuntimeWorkflowStepStatus =
  | "PENDING"
  | "RUNNING"
  | "WAITING_USER_APPROVAL"
  | "DONE"
  | "FAILED"
  | "SKIPPED";

export interface RuntimeWorkflowStep {
  id: string;
  stepType: string;
  status: RuntimeWorkflowStepStatus;
  requiresApproval: boolean;
  startedAt: string | null;
  retryCount: number;
  rejectionNote: string | null;
}

export interface WorkflowStepPatch {
  status: RuntimeWorkflowStepStatus;
  startedAt?: string | null;
  finishedAt?: string | null;
  rejectionNote?: string | null;
  retryCount?: number;
}

export interface WorkflowStepTransition {
  stepId: string;
  patch: WorkflowStepPatch;
  logs: Array<{
    logLevel: "info" | "warn" | "error" | "debug";
    message: string;
  }>;
}

export interface WorkflowProgressPlan {
  runStatus: "RUNNING" | "DONE";
  runFinishedAt: string | null;
  pausedStepId: string | null;
  stepTransitions: WorkflowStepTransition[];
}

export function planWorkflowProgress({
  steps,
  yoloMode,
  now,
}: {
  steps: RuntimeWorkflowStep[];
  yoloMode: boolean;
  now: string;
}): WorkflowProgressPlan {
  const stepTransitions: WorkflowStepTransition[] = [];

  for (const step of steps) {
    if (step.status === "DONE" || step.status === "SKIPPED" || step.status === "FAILED") {
      continue;
    }

    if (step.status === "WAITING_USER_APPROVAL") {
      if (!yoloMode) {
        return {
          runStatus: "RUNNING",
          runFinishedAt: null,
          pausedStepId: step.id,
          stepTransitions,
        };
      }

      stepTransitions.push({
        stepId: step.id,
        patch: {
          status: "DONE",
          startedAt: step.startedAt ?? now,
          finishedAt: now,
          rejectionNote: null,
        },
        logs: [
          {
            logLevel: "info",
            message: `YOLO mode auto-approved step ${step.stepType}.`,
          },
        ],
      });
      continue;
    }

    if (step.status === "PENDING" || step.status === "RUNNING") {
      if (step.requiresApproval && !yoloMode) {
        stepTransitions.push({
          stepId: step.id,
          patch: {
            status: "WAITING_USER_APPROVAL",
            startedAt: step.startedAt ?? now,
            finishedAt: null,
          },
          logs: [
            {
              logLevel: "info",
              message: `Generation complete. Paused on approval gate for ${step.stepType}.`,
            },
          ],
        });

        return {
          runStatus: "RUNNING",
          runFinishedAt: null,
          pausedStepId: step.id,
          stepTransitions,
        };
      }

      stepTransitions.push({
        stepId: step.id,
        patch: {
          status: "DONE",
          startedAt: step.startedAt ?? now,
          finishedAt: now,
          rejectionNote: null,
        },
        logs: [
          {
            logLevel: "info",
            message:
              yoloMode && step.requiresApproval
                ? `YOLO mode auto-approved step ${step.stepType}.`
                : `Completed step ${step.stepType}.`,
          },
        ],
      });
    }
  }

  return {
    runStatus: "DONE",
    runFinishedAt: now,
    pausedStepId: null,
    stepTransitions,
  };
}

export function planRejectedStepRetry({
  stepId,
  step,
  now,
  comment,
}: {
  stepId: string;
  step: Pick<RuntimeWorkflowStep, "status" | "retryCount">;
  now: string;
  comment: string;
}): WorkflowStepTransition {
  return {
    stepId,
    patch: {
      status: "PENDING",
      startedAt: null,
      finishedAt: null,
      rejectionNote: comment,
      retryCount: step.retryCount + 1,
    },
    logs: [
      {
        logLevel: "warn",
        message: `Rejection received at ${now}: "${comment}". Resetting step for another pass.`,
      },
    ],
  };
}
