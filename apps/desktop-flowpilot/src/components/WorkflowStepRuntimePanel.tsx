import { useEffect } from "react";
import { isFlowModeRun, useStore } from "@/state/store";
import type { WorkflowStepRuntimeDTO, WorkflowStepRuntimeStatus } from "@/types/contract";

// BUG-153: Flow mode already runs on deterministic Go-owned workflow steps, but the
// right sidebar showed only generic chat panels. This panel projects the Go
// orchestrator's runtime step list (workflowStore.LoadRunSteps) so users can see
// at a glance which step is active, which are done, and whether a step is
// retrying or blocked — without reconstructing it from chat text (F-10..F-15).

const STATUS_DOT_CLASS: Record<WorkflowStepRuntimeStatus, string> = {
  PENDING: "closed",
  RUNNING: "run",
  WAITING_USER_APPROVAL: "wait",
  DONE: "done",
  FAILED: "fail",
  SKIPPED: "closed",
};

const STATUS_LABEL: Record<WorkflowStepRuntimeStatus, string> = {
  PENDING: "pending",
  RUNNING: "running",
  WAITING_USER_APPROVAL: "waiting for approval",
  DONE: "done",
  FAILED: "failed",
  SKIPPED: "skipped",
};

function stepLabel(step: WorkflowStepRuntimeDTO): string {
  // BUG-155: for a CP-42 flow-engine node, stepType is a shared generic
  // dispatch category (e.g. "flow-agent-delegate") identical across every
  // node running the same behavior — nodeId is the actual per-step name.
  return step.nodeId || step.stepType || step.stepId;
}

export function WorkflowStepRuntimePanel(): React.ReactElement | null {
  const chatMode = useStore((s) => s.chatMode);
  const mainRunId = useStore((s) => s.mainRunId ?? s.runId);
  const steps = useStore((s) => s.workflowStepRuntime);
  const loading = useStore((s) => s.workflowStepRuntimeLoading);
  const refreshWorkflowStepRuntime = useStore((s) => s.refreshWorkflowStepRuntime);

  useEffect(() => {
    void refreshWorkflowStepRuntime();
  }, [refreshWorkflowStepRuntime, mainRunId, chatMode]);

  // F-11: only Flow mode / workflow-step-backed runs get this panel. Review Loop
  // and other normal-chat orchestration keep their existing chat/agent feel.
  if (!isFlowModeRun(chatMode)) return null;

  return (
    <section className="panel workflow-step-runtime">
      <div className="panel-h">
        <span>Workflow Steps</span>
        {loading && <span className="acount">loading…</span>}
      </div>
      <p className="panel-sub">Step-by-step runtime state from the Go workflow engine.</p>

      {steps.length === 0 ? (
        <div className="wsr-empty">
          {loading ? "Loading step state…" : "No step-runtime data for this run yet."}
        </div>
      ) : (
        <div className="agent-run-list" style={{ display: "flex", flexDirection: "column", gap: "6px", padding: 0 }}>
          {steps.map((step, index) => (
            <div key={step.stepId} className="acard wsr-step">
              <div className="ac-top">
                <span className="ac-nm">
                  {index + 1}. {stepLabel(step)}
                </span>
                <span className="ac-st">
                  <span className={`sd ${STATUS_DOT_CLASS[step.status]}`} />
                  {STATUS_LABEL[step.status]}
                </span>
              </div>
              <div className="ac-meta">
                {step.provider && (
                  <span className={`pill-prov prov-${step.provider}`}>{step.provider.toUpperCase()}</span>
                )}
                {step.model && <span className="ac-model">{step.model}</span>}
                {step.agentRef && <span>agent: {step.agentRef}</span>}
                {step.yoloMode && <span>yolo</span>}
                {step.retryCount > 0 && <span className="wsr-retry-badge">Retry {step.retryCount}</span>}
                {step.requiresApproval && <span>requires approval</span>}
              </div>
              {step.rejectionNote && <div className="ac-last">{step.rejectionNote}</div>}
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
