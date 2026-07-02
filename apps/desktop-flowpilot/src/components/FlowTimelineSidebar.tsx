import { useEffect, useState } from "react";
import { FlowStepTimeline } from "@/components/FlowStepTimeline";
import { activeWorkflowStep, isFlowModeRun, useStore } from "@/state/store";

// BUG-156: dedicated collapsible middle sidebar for Flow Mode's step timeline,
// carved out of the right-sidebar-stack (where WorkflowStepRuntimePanel used to
// live) so it can sit between the chat column and the existing right sidebar and
// only take up space while a flow is actually running. Collapsed shows just the
// icon rail; expanded adds a progress summary header plus full step detail
// (name/status/provider/model/agent/yolo — BUG-155).
export function FlowTimelineSidebar(): React.ReactElement | null {
  const chatMode = useStore((s) => s.chatMode);
  const mainRunId = useStore((s) => s.mainRunId ?? s.runId);
  const steps = useStore((s) => s.workflowStepRuntime);
  const meta = useStore((s) => s.workflowStepRuntimeMeta);
  const refreshWorkflowStepRuntime = useStore((s) => s.refreshWorkflowStepRuntime);
  const [expanded, setExpanded] = useState(true);

  // BUG-175: once a flow run has actually started, keep the step-timeline
  // sidebar mounted for its entire lifetime — running, paused on an approval or
  // question, errored, or completed. The timeline is orientation the user needs
  // in every one of those states. Gating on runStatus (BUG-168) still lost it
  // whenever the *main* run went idle — e.g. while a sub-agent holds an
  // approval (the main run isn't "running" then), between the coder finishing
  // and the reviewers starting, or once the run completed. A flow run either
  // exists (mainRunId set, or step data loaded) or it doesn't; status is not a
  // visibility signal. The only hidden case is before the first prompt is sent,
  // when no run exists yet — resetRun clears both mainRunId and the step list.
  const runStarted = Boolean(mainRunId) || steps.length > 0;
  const visible = isFlowModeRun(chatMode) && runStarted;

  useEffect(() => {
    if (!visible) return;
    void refreshWorkflowStepRuntime();
  }, [visible, refreshWorkflowStepRuntime, mainRunId]);

  if (!visible) return null;

  const current = activeWorkflowStep(steps);
  // BUG-159: "reached" (done, or currently on it), not just "fully done" — the
  // user is standing ON step 1 while it runs, so that should read "1/4", not "0/4".
  const reachedCount = steps.filter(
    (s) => s.status === "DONE" || s.status === "RUNNING" || s.status === "WAITING_USER_APPROVAL",
  ).length;

  return (
    <aside className={`flow-sidebar ${expanded ? "flow-sidebar-expanded" : "flow-sidebar-collapsed"}`}>
      <div className="flow-sidebar-head">
        {expanded && (
          <div className="flow-sidebar-summary">
            <span className="flow-sidebar-progress">
              {reachedCount}/{steps.length} steps
            </span>
            {current && <span className="flow-sidebar-current">{current.nodeId || current.stepType}</span>}
            <span className="flow-sidebar-meta">
              {meta.provider && (
                <span className={`pill-prov prov-${meta.provider}`}>{meta.provider.toUpperCase()}</span>
              )}
              {meta.model && <span className="ac-model">{meta.model}</span>}
              {/* BUG-159: yolo is a run-wide toggle, not a per-step config, so it's
                  surfaced once here — always visible (on or off), not only when on,
                  so the user can tell the flow's yolo posture at a glance. */}
              <span className={`wsr-retry-badge ${meta.yoloMode ? "yolo-on" : "yolo-off"}`}>
                YOLO {meta.yoloMode ? "ON" : "OFF"}
              </span>
            </span>
          </div>
        )}
        <button
          type="button"
          className="flow-sidebar-toggle"
          aria-label={expanded ? "Collapse step timeline" : "Expand step timeline"}
          aria-expanded={expanded}
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? "❮" : "❯"}
        </button>
      </div>

      <div className="flow-sidebar-body">
        {steps.length === 0 ? (
          expanded && <div className="wsr-empty">No step-runtime data for this run yet.</div>
        ) : (
          <FlowStepTimeline steps={steps} compact={!expanded} runProvider={meta.provider} runModel={meta.model} />
        )}
      </div>
    </aside>
  );
}
