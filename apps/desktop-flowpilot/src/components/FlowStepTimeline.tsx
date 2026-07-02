import type { WorkflowStepRuntimeDTO, WorkflowStepRuntimeStatus } from "@/types/contract";

// BUG-156: vertical connected-circle stepper for Flow Mode's step list, replacing
// WorkflowStepRuntimePanel's flat card list. Each icon encodes one of five visual
// states (idle/running/done/approval/error); the step currently RUNNING or
// WAITING_USER_APPROVAL gets its whole row highlighted so it reads as "you are here"
// at a glance, matching BUG-153's original "see at a glance which step is active" goal.

type TimelineVisualState = "idle" | "running" | "done" | "approval" | "error";

function visualState(status: WorkflowStepRuntimeStatus): TimelineVisualState {
  switch (status) {
    case "RUNNING":
      return "running";
    case "DONE":
      return "done";
    case "WAITING_USER_APPROVAL":
      return "approval";
    case "FAILED":
      return "error";
    case "PENDING":
    case "SKIPPED":
    default:
      return "idle";
  }
}

const STATE_GLYPH: Record<TimelineVisualState, string> = {
  idle: "",
  running: "",
  done: "✓",
  approval: "!",
  error: "✕",
};

const STATE_LABEL: Record<WorkflowStepRuntimeStatus, string> = {
  PENDING: "pending",
  RUNNING: "running",
  WAITING_USER_APPROVAL: "waiting for approval",
  DONE: "done",
  FAILED: "failed",
  SKIPPED: "skipped",
};

function stepName(step: WorkflowStepRuntimeDTO): string {
  // nodeId is the flow-graph node id ("coder", "reviewer_correctness"); stepType
  // for a CP-42 flow-engine node is a shared generic dispatch category
  // ("flow-agent-delegate") identical across every node on the same behavior, so
  // it is only a fallback for steps that predate node_id (BUG-155).
  return step.nodeId || step.stepType || step.stepId;
}

interface FlowStepTimelineProps {
  steps: WorkflowStepRuntimeDTO[];
  /** Icon-only rail: no title/description/meta, just the connected state dots. */
  compact?: boolean;
}

export function FlowStepTimeline({ steps, compact = false }: FlowStepTimelineProps): React.ReactElement {
  return (
    <ol className={`flow-timeline ${compact ? "flow-timeline-compact" : ""}`}>
      {steps.map((step, index) => {
        const state = visualState(step.status);
        const isCurrent = state === "running" || state === "approval";
        const isLast = index === steps.length - 1;
        const lineState = state === "done" ? "done" : state === "running" ? "running" : "idle";

        return (
          <li
            key={step.stepId}
            className={`flow-timeline-item fti-${state} ${isCurrent ? "fti-current" : ""}`}
            title={compact ? `${stepName(step)} — ${STATE_LABEL[step.status]}` : undefined}
          >
            <div className="fti-track">
              <span className="fti-icon">{STATE_GLYPH[state]}</span>
              {!isLast && <span className={`fti-line fti-line-${lineState}`} />}
            </div>

            {!compact && (
              <div className="fti-body">
                <div className="fti-title">{stepName(step)}</div>
                <div className="fti-desc">
                  {step.rejectionNote || STATE_LABEL[step.status]}
                  {step.retryCount > 0 && <span className="wsr-retry-badge">Retry {step.retryCount}</span>}
                </div>
                {(step.provider || step.model || step.agentRef || step.yoloMode) && (
                  <div className="fti-meta">
                    {step.provider && (
                      <span className={`pill-prov prov-${step.provider}`}>{step.provider.toUpperCase()}</span>
                    )}
                    {step.model && <span className="ac-model">{step.model}</span>}
                    {step.agentRef && <span>agent: {step.agentRef}</span>}
                    {step.yoloMode && <span>yolo</span>}
                  </div>
                )}
              </div>
            )}
          </li>
        );
      })}
    </ol>
  );
}
