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
  const runStatus = useStore((s) => s.status);
  const mainRunId = useStore((s) => s.mainRunId ?? s.runId);
  const steps = useStore((s) => s.workflowStepRuntime);
  const refreshWorkflowStepRuntime = useStore((s) => s.refreshWorkflowStepRuntime);
  const [expanded, setExpanded] = useState(true);

  const visible = isFlowModeRun(chatMode) && runStatus === "running";

  useEffect(() => {
    if (!visible) return;
    void refreshWorkflowStepRuntime();
  }, [visible, refreshWorkflowStepRuntime, mainRunId]);

  if (!visible) return null;

  const current = activeWorkflowStep(steps);
  const doneCount = steps.filter((s) => s.status === "DONE").length;

  return (
    <aside className={`flow-sidebar ${expanded ? "flow-sidebar-expanded" : "flow-sidebar-collapsed"}`}>
      <div className="flow-sidebar-head">
        {expanded && (
          <div className="flow-sidebar-summary">
            <span className="flow-sidebar-progress">
              {doneCount}/{steps.length} steps
            </span>
            {current && <span className="flow-sidebar-current">{current.nodeId || current.stepType}</span>}
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
          <FlowStepTimeline steps={steps} compact={!expanded} />
        )}
      </div>
    </aside>
  );
}
