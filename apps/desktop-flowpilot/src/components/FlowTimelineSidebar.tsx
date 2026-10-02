import { useEffect, useRef, useState } from "react";
import { FlowStepTimeline } from "@/components/FlowStepTimeline";
import { activeWorkflowStep, isFlowModeRun, useStore } from "@/state/store";
import {
  activityCountsForStatus,
  formatActivityAge,
  lastActivityAt,
  runActivityKind,
} from "@/state/runActivity";
import { ChevronLeftIcon, ChevronRightIcon } from "@/components/icons";

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
  // CA-1097 (live run-3362): vibe-tasks runs 1 CP → N existing Tasks, but the
  // desktop gave no hint which task the sprint was on — the TUI already renders
  // a "task i/total <name>" chip. Mirror it from loopState.vibeTask*.
  const vibeTaskIndex = useStore((s) => s.agentGraphSnapshot?.loopState?.vibeTaskIndex ?? 0);
  const vibeTaskTotal = useStore((s) => s.agentGraphSnapshot?.loopState?.vibeTaskTotal ?? 0);
  const vibeTaskName = useStore((s) => s.agentGraphSnapshot?.loopState?.vibeTaskName ?? "");
  // Task-458: liveness is stamped per run id on every live SSE arrival
  // (store.lastActivityByRun). Roll the family up — main run + non-terminal
  // legs — so a focused child mid-exec and a hub turn both keep the chip warm.
  const lastActivityByRun = useStore((s) => s.lastActivityByRun);
  const agentRuns = useStore((s) => s.agentRuns);
  const [expanded, setExpanded] = useState(true);
  // Auto-collapse the step rail on narrow windows; restores the user's choice
  // when the window widens back past the breakpoint.
  const remembered = useRef<boolean | undefined>(undefined);
  const expandedRef = useRef(expanded);
  expandedRef.current = expanded;
  useEffect(() => {
    const mq = window.matchMedia("(max-width: 1180px)");
    const sync = () => {
      if (mq.matches) {
        if (remembered.current === undefined) remembered.current = expandedRef.current;
        setExpanded(false);
      } else if (remembered.current !== undefined) {
        setExpanded(remembered.current);
        remembered.current = undefined;
      }
    };
    sync();
    mq.addEventListener("change", sync);
    return () => mq.removeEventListener("change", sync);
  }, []);

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

  // Task-458: 1s local tick ages the liveness chip. The interval only runs
  // while a step is RUNNING and the sidebar is visible — an idle, parked or
  // terminal flow never burns a render loop.
  const hasRunningStep = steps.some((s) => s.status === "RUNNING");
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!visible || !hasRunningStep) return;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [visible, hasRunningStep]);

  if (!visible) return null;

  const current = activeWorkflowStep(steps);
  const activeRunIds = [
    ...(mainRunId ? [mainRunId] : []),
    ...agentRuns.filter((r) => activityCountsForStatus(r.status)).map((r) => r.runId),
  ];
  const liveLastAt = lastActivityAt(lastActivityByRun, activeRunIds);
  const liveKind = liveLastAt !== undefined ? runActivityKind(liveLastAt, now) : undefined;
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
            {/* CA-1097: which of the CP's pre-existing Tasks the sprint is on —
                mirrors the TUI vibeTaskChip. */}
            {vibeTaskTotal > 0 && vibeTaskIndex > 0 && (
              <span className="flow-sidebar-task" title={vibeTaskName || undefined}>
                Task {vibeTaskIndex}/{vibeTaskTotal}
                {vibeTaskName ? ` — ${vibeTaskName}` : ""}
              </span>
            )}
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
            {/* Task-458: freshness of the run's own SSE stream — no log text.
                Rendered only while a step is RUNNING; a step parked on an
                approval/question is not "live" even though it is active. */}
            {current?.status === "RUNNING" && liveLastAt !== undefined && liveKind && (
              <span
                className={`flow-sidebar-live flow-sidebar-live-${liveKind}`}
                title="Age of the last event received on this run's stream"
              >
                {liveKind === "live"
                  ? `live — last event ${formatActivityAge(now - liveLastAt)} ago`
                  : `quiet ${formatActivityAge(now - liveLastAt)}`}
              </span>
            )}
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
          {expanded ? <ChevronLeftIcon size={11} /> : <ChevronRightIcon size={11} />}
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
