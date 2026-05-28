type WorkflowRunLike = {
  status: string | null | undefined;
};

type WorkflowStepLike = {
  id: string;
  status: string | null | undefined;
};

type WorkflowSessionLike = {
  status: string | null | undefined;
  processKey: string | null | undefined;
};

export const INTERRUPTED_RUN_ERROR =
  "Runner session ended before this workflow step finished. Start a new replay session to continue.";

function normalize(value: string | null | undefined) {
  return (value ?? "").trim().toLowerCase();
}

export function canResumeWorkflowRun({
  run,
  steps,
}: {
  run: WorkflowRunLike | null | undefined;
  steps: WorkflowStepLike[] | null | undefined;
}) {
  const runStatus = normalize(run?.status);
  if (
    runStatus === "completed" ||
    runStatus === "done" ||
    runStatus === "rejected" ||
    runStatus === "canceled" ||
    runStatus === "failed"
  ) {
    return false;
  }

  return (steps ?? []).some((step) => normalize(step.status) === "pending");
}

export function canCancelWorkflowRun(run: WorkflowRunLike | null | undefined) {
  const runStatus = normalize(run?.status);
  return (
    runStatus === "draft" ||
    runStatus === "pending" ||
    runStatus === "running" ||
    runStatus === "waiting_approval"
  );
}

export function getInterruptedWorkflowRunStepIds({
  run,
  steps,
  sessions,
}: {
  run: WorkflowRunLike | null | undefined;
  steps: WorkflowStepLike[] | null | undefined;
  sessions: WorkflowSessionLike[] | null | undefined;
}) {
  const runStatus = normalize(run?.status);
  if (runStatus !== "running") {
    return [];
  }

  if (!sessions || sessions.length === 0) {
    return [];
  }

  const hasLiveActiveSession = (sessions ?? []).some(
    (session) =>
      normalize(session.status) === "active" &&
      (session.processKey?.trim().length ?? 0) > 0,
  );
  if (hasLiveActiveSession) {
    return [];
  }

  return (steps ?? [])
    .filter((step) => normalize(step.status) === "running")
    .map((step) => step.id);
}

export function isInterruptedWorkflowStep(step: {
  status: string | null | undefined;
  errorMessage?: string | null | undefined;
}) {
  return (
    normalize(step.status) === "failed" &&
    normalize(step.errorMessage) === normalize(INTERRUPTED_RUN_ERROR)
  );
}
