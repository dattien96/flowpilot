import { useEffect, useState } from "react";
import { FlowStepTimeline } from "@/components/FlowStepTimeline";
import { activeWorkflowStep, isFlowModeRun, useStore } from "@/state/store";

// BUG-156: dedicated collapsible middle sidebar for Flow Mode's step timeline,
// carved out of the right-sidebar-stack (where WorkflowStepRuntimePanel used to
// live) so it can sit between the chat column and the existing right sidebar and
// only take up space while a flow is actually running. Collapsed shows just the
// icon rail; expanded adds a progress summary header plus full step detail
// (name/status/provider/model/agent — BUG-155). YOLO is not shown: Flow/Workflow
// launches always run YOLO=true (WorkflowsSettings lock); a stale meta.yoloMode=false
// was rendering "YOLO OFF" which is wrong and noise.
export function FlowTimelineSidebar(): React.ReactElement | null {
  const chatMode = useStore((s) => s.chatMode);
  const mainRunId = useStore((s) => s.mainRunId ?? s.runId);
  const steps = useStore((s) => s.workflowStepRuntime);
  const meta = useStore((s) => s.workflowStepRuntimeMeta);
  const refreshWorkflowStepRuntime = useStore((s) => s.refreshWorkflowStepRuntime);
  // BUG-234 (#2): the review loop can send the flow back to the coder for another
  // round. The step timeline reuses one row per node, so a loop-back is otherwise
  // invisible (the same 4 rows just re-run). Surface the loop's round counter so a
  // loop-back reads as "Round 2/3" rather than looking like the first pass repeating.
  const loopRound = useStore((s) => s.agentGraphSnapshot?.loopState?.round ?? 0);
  const loopCap = useStore((s) => {
    const ls = s.agentGraphSnapshot?.loopState;
    return ls?.cap ?? ls?.roundCap ?? 0;
  });
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
    (s) =>
      s.status === "DONE" ||
      s.status === "RUNNING" ||
      s.status === "WAITING_USER_APPROVAL" ||
      s.status === "FAILED" ||
      s.status === "CANCELED",
  ).length;

  return (
    <aside className={`flow-sidebar ${expanded ? "flow-sidebar-expanded" : "flow-sidebar-collapsed"}`}>
      <div className="flow-sidebar-head">
        {expanded && (
          <div className="flow-sidebar-summary">
            {/* BUG-234 (#2): round counter — loopRound is 0-based (0 = first pass),
                so display round+1. Highlighted once a loop-back has occurred so the
                user can see the flow returned to the coder for another round. */}
            {steps.length > 0 && (
              <span className={`flow-sidebar-round ${loopRound > 0 ? "flow-sidebar-round-active" : ""}`}>
                Round {loopRound + 1}
                {loopCap > 0 ? `/${loopCap}` : ""}
              </span>
            )}
            <span className="flow-sidebar-progress">
              {reachedCount}/{steps.length} steps
            </span>
            {current && <span className="flow-sidebar-current">{current.nodeId || current.stepType}</span>}
            <span className="flow-sidebar-meta">
              {meta.provider && (
                <span className={`pill-prov prov-${meta.provider}`}>{meta.provider.toUpperCase()}</span>
              )}
              {meta.model && <span className="ac-model">{meta.model}</span>}
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
