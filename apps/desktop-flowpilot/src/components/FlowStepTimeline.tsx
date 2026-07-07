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

// A node's agentRef is stored as either a bare agent name or a full definition
// path (the Agent-ref dropdown stores agent.path to disambiguate same-named
// files across sources). The timeline only needs the human-readable name, which
// is what the runtime resolves the ref to — so collapse a path to its base name
// without extension, matching the clean name shown in the Agents panel.
function agentRefLabel(agentRef: string): string {
  const base = agentRef.split(/[\\/]/).pop() ?? agentRef;
  const dot = base.lastIndexOf(".");
  return dot > 0 ? base.slice(0, dot) : base;
}

function stepName(step: WorkflowStepRuntimeDTO): string {
  // nodeId is the flow-graph node id ("coder", "reviewer_correctness"); stepType
  // for a CP-42 flow-engine node is a shared generic dispatch category
  // ("flow-agent-delegate") identical across every node on the same behavior, so
  // it is only a fallback for steps that predate node_id (BUG-155).
  return step.nodeId || step.stepType || step.stepId;
}

interface FlowStepTimelineProps {
  steps: WorkflowStepRuntimeDTO[];
  /** Icon-only rail: no title/description/meta, just the connected state dots (numbered — BUG-158). */
  compact?: boolean;
  /**
   * Run-level provider/model fallback (BUG-158): a flow node's agent
   * definition has no override of its own for most built-in flows, so it
   * inherits whatever the run itself was started with. Shown per step only
   * when the step doesn't declare its own provider_override/model_override.
   */
  runProvider?: string;
  runModel?: string;
}

export function FlowStepTimeline({
  steps,
  compact = false,
  runProvider,
  runModel,
}: FlowStepTimelineProps): React.ReactElement {
  return (
    <ol className={`flow-timeline ${compact ? "flow-timeline-compact" : ""}`}>
      {steps.map((step, index) => {
        const state = visualState(step.status);
        const isCurrent = state === "running" || state === "approval";
        const isLast = index === steps.length - 1;
        const lineState = state === "done" ? "done" : state === "running" ? "running" : "idle";
        const provider = step.provider || runProvider;
        const model = step.model || runModel;

        return (
          <li
            key={step.stepId}
            className={`flow-timeline-item fti-${state} ${isCurrent ? "fti-current" : ""}`}
            title={compact ? `${stepName(step)} — ${STATE_LABEL[step.status]}` : undefined}
          >
            <div className="fti-track">
              {/* BUG-173: number every step 1-2-3-4 in BOTH modes so the expanded
                  rail matches the collapsed rail. Status is already conveyed by the
                  fti-{state} color classes and the current-step highlight, so the
                  step index is the more useful glyph than the sparse STATE_GLYPH set
                  (which was empty for idle/running — leaving expanded circles blank).
                  Done/error keep their ✓/✕ mark, which reads as an at-a-glance
                  completion cue layered on top of the ordering the numbers give. */}
              <span className="fti-icon">
                {state === "done" || state === "error" ? STATE_GLYPH[state] : index + 1}
              </span>
              {!isLast && <span className={`fti-line fti-line-${lineState}`} />}
            </div>

            {!compact && (
              <div className="fti-body">
                <div className="fti-title">{stepName(step)}</div>
                <div className="fti-desc">
                  {step.rejectionNote || STATE_LABEL[step.status]}
                  {step.retryCount > 0 && <span className="wsr-retry-badge">Retry {step.retryCount}</span>}
                </div>
                {(provider || model || step.agentRef || step.yoloMode) && (
                  <div className="fti-meta">
                    {provider && <span className={`pill-prov prov-${provider}`}>{provider.toUpperCase()}</span>}
                    {model && <span className="ac-model">{model}</span>}
                    {step.agentRef && <span title={step.agentRef}>agent: {agentRefLabel(step.agentRef)}</span>}
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
