import { useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, PointerEvent as ReactPointerEvent } from "react";
import { filterNavigatorWorkflows } from "@/app/navigatorCatalog";
import { Navigator } from "@/components/Navigator";
import { ChatInput } from "@/components/ChatInput";
import { TerminalPanel } from "@/components/TerminalPanel";
import { SpectatorPane } from "@/components/SpectatorPane";
import { Timeline } from "@/components/Timeline";
import { ScenarioSwitcher } from "@/components/ScenarioSwitcher";
import { SystemControls } from "@/components/SystemControls";
import { ProviderAccountsPanel } from "@/components/ProviderAccountsPanel";
import { AgentsPanel } from "@/components/AgentsPanel";
import { FlowTimelineSidebar } from "@/components/FlowTimelineSidebar";
import { OrchestrationBoard } from "@/components/OrchestrationBoard";
import { FlowAwaitingUserCard } from "@/components/FlowAwaitingUserCard";
import { DispatchAttentionCard } from "@/components/DispatchAttentionCard";
import { ChatPosturePanel } from "@/components/ChatPosturePanel";
import { ChatBootOverlay } from "@/components/ChatBootOverlay";
import { LSPStatusNotice } from "@/components/LSPStatusNotice";
import { gateBlockSecondaryAction } from "@/components/gateBlockActions";
import { GateIcon, WarnIcon } from "@/components/icons";
import { useStore, accountLabel, providerLabel } from "@/state/store";

function WorkflowControlPanel(): React.ReactElement | null {
  const selectedProjectId = useStore((s) => s.selectedProjectId);
  const chatMode = useStore((s) => s.chatMode);
  const launchMode = useStore((s) => s.launchMode);
  const workflows = useStore((s) => s.workflows);
  const steps = useStore((s) => s.steps);
  const selectedWorkflowId = useStore((s) => s.selectedWorkflowId);
  const selectedStepId = useStore((s) => s.selectedStepId);
  const setChatMode = useStore((s) => s.setChatMode);
  const setLaunchMode = useStore((s) => s.setLaunchMode);
  const selectWorkflow = useStore((s) => s.selectWorkflow);
  const selectStep = useStore((s) => s.selectStep);
  const projects = useStore((s) => s.projects);

  const project = useMemo(() => projects.find((p) => p.id === selectedProjectId), [projects, selectedProjectId]);
  const visibleWorkflows = useMemo(
    () => filterNavigatorWorkflows(workflows, selectedProjectId),
    [selectedProjectId, workflows],
  );

  return (
    <section className="workflow-rail workflow-rail-right">
      <div className="project-rail-head">
        <div>
          <label>Mode</label>
          <p>Switch between direct chat and workflow launch selection.</p>
        </div>
      </div>

      <div className="tab-list" role="tablist" aria-label="Chat mode">
        {(["normal_chat", "workflow_step_auto"] as const).map((mode) => (
          <button
            key={mode}
            type="button"
            role="tab"
            aria-selected={chatMode === mode}
            className={`tab ${chatMode === mode ? "active" : ""}`}
            onClick={() => setChatMode(mode)}
          >
            {mode === "normal_chat" ? "Chat" : "Workflow"}
          </button>
        ))}
      </div>

      {chatMode === "workflow_step_auto" && (
        <>
          <div className="nav-group nav-group-inline">
            <label>Run type</label>
            <div className="tab-list" role="tablist" aria-label="Run type">
              <button
                type="button"
                role="tab"
                aria-selected={launchMode === "workflow"}
                aria-controls="workflow-panel"
                className={`tab ${launchMode === "workflow" ? "active" : ""}`}
                onClick={() => setLaunchMode("workflow")}
              >
                Workflow
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={launchMode === "step"}
                aria-controls="step-panel"
                className={`tab ${launchMode === "step" ? "active" : ""}`}
                onClick={() => setLaunchMode("step")}
              >
                Single step
              </button>
            </div>
          </div>

          <div
            id="workflow-panel"
            role="tabpanel"
            aria-hidden={launchMode !== "workflow"}
            className={`nav-panel ${launchMode === "workflow" ? "active" : "hidden"}`}
          >
            <div className="nav-group">
              <label>Workflow</label>
              <select
                value={selectedWorkflowId ?? ""}
                disabled={!selectedProjectId || launchMode !== "workflow"}
                onChange={(e) => void selectWorkflow(e.target.value)}
              >
                <option value="" disabled>
                  Select a workflow…
                </option>
                {visibleWorkflows.map((workflow) => {
                  const resolves = Boolean(workflow.model || project?.model);
                  return (
                    <option key={workflow.id} value={workflow.id} disabled={!resolves}>
                      {workflow.name} {!resolves ? " (No model set)" : ""}
                    </option>
                  );
                })}
              </select>
            </div>
          </div>

          <div
            id="step-panel"
            role="tabpanel"
            aria-hidden={launchMode !== "step"}
            className={`nav-panel ${launchMode === "step" ? "active" : "hidden"}`}
          >
            <div className="nav-group">
              <label>Single step</label>
              <select
                value={selectedStepId ?? ""}
                disabled={!selectedProjectId || launchMode !== "step"}
                onChange={(e) => selectStep(e.target.value)}
              >
                <option value="" disabled>
                  Select a step…
                </option>
                {steps.map((step) => {
                  const resolves = Boolean(step.model || project?.model);
                  return (
                    <option key={step.id} value={step.id} disabled={!resolves}>
                      {step.name} {!resolves ? " (No model set)" : ""}
                    </option>
                  );
                })}
              </select>
            </div>
          </div>
        </>
      )}
    </section>
  );
}

interface ChatWorkspaceProps {
  leftSidebarVisible: boolean;
  rightSidebarVisible: boolean;
}

function AccountSwitchModal(): React.ReactElement | null {
  const pendingAccountSwitch = useStore((s) => s.pendingAccountSwitch);
  const accountSwitchLoading = useStore((s) => s.accountSwitchLoading);
  const confirmAccountSwitch = useStore((s) => s.confirmAccountSwitch);
  const cancelAccountSwitch = useStore((s) => s.cancelAccountSwitch);

  if (!pendingAccountSwitch && !accountSwitchLoading) return null;

  return (
    <div
      className="account-switch-overlay"
      role="dialog"
      aria-modal="true"
      aria-label="Switch account"
      onClick={!accountSwitchLoading ? cancelAccountSwitch : undefined}
    >
      <div className="account-switch-modal" onClick={(e) => e.stopPropagation()}>
        {accountSwitchLoading ? (
          <p className="account-switch-loading">Switching account...</p>
        ) : pendingAccountSwitch ? (
          <>
            <p className="account-switch-reason">
              {pendingAccountSwitch.reason === "manual"
                ? "Switch to a better account"
                : "Current account hit usage limit"}
            </p>
            <div className="account-switch-details">
              <div className="account-switch-row">
                <span className="account-switch-row-label">Provider</span>
                <span>{providerLabel(pendingAccountSwitch.providerKey)}</span>
              </div>
              <div className="account-switch-row">
                <span className="account-switch-row-label">Current</span>
                <span>{pendingAccountSwitch.failedAccountLabel}</span>
              </div>
              <div className="account-switch-row">
                <span className="account-switch-row-label">Switch to</span>
                <span>{accountLabel(pendingAccountSwitch.candidateAccount)}</span>
              </div>
            </div>
            <div className="account-switch-actions">
              <button type="button" className="project-history-confirm-cancel" onClick={cancelAccountSwitch}>
                Cancel
              </button>
              <button type="button" className="project-history-confirm-ok" onClick={() => void confirmAccountSwitch()}>
                {pendingAccountSwitch.reason === "manual" ? "Switch" : "Switch and retry"}
              </button>
            </div>
          </>
        ) : null}
      </div>
    </div>
  );
}

// GrokYoloPostureModal (Task-218): unlike Claude/Codex/Gemini's YOLO toggle
// (a synchronous local flip), Grok's requires the backend to rewrite the
// active account's config.toml and respawn its shared process for gating to
// actually take effect -- shown for however long toggleYoloForActiveProvider's
// applyGrokYoloPosture call takes. Non-dismissible while loading, mirroring
// AccountSwitchModal's own overlay/modal shell (no cancel path -- there's
// nothing to cancel, the call is already in flight).
function GrokYoloPostureModal(): React.ReactElement | null {
  const grokYoloPostureLoading = useStore((s) => s.grokYoloPostureLoading);
  if (!grokYoloPostureLoading) return null;

  return (
    <div className="account-switch-overlay" role="dialog" aria-modal="true" aria-label="Applying Grok YOLO setting">
      <div className="account-switch-modal">
        <p className="account-switch-loading">Applying YOLO setting for Grok…</p>
      </div>
    </div>
  );
}

function ProviderSwitchModal(): React.ReactElement | null {
  const pendingProviderSwitch = useStore((s) => s.pendingProviderSwitch);
  const providerSwitchLoading = useStore((s) => s.providerSwitchLoading);
  const confirmProviderSwitch = useStore((s) => s.confirmProviderSwitch);
  const cancelProviderSwitch = useStore((s) => s.cancelProviderSwitch);

  if (!pendingProviderSwitch && !providerSwitchLoading) return null;

  return (
    <div
      className="account-switch-overlay"
      role="dialog"
      aria-modal="true"
      aria-label="Switch provider"
      onClick={!providerSwitchLoading ? cancelProviderSwitch : undefined}
    >
      <div className="account-switch-modal" onClick={(e) => e.stopPropagation()}>
        {providerSwitchLoading ? (
          <p className="account-switch-loading">Starting a new chat with the selected provider...</p>
        ) : pendingProviderSwitch ? (
          <>
            <p className="account-switch-reason">Switch providers for this chat?</p>
            <p className="gate-block-detail" style={{ marginTop: 0 }}>
              FlowPilot will build a bounded context handoff from the source run and start a new run.
              The source chat stays in history.
            </p>
            <div className="account-switch-details">
              <div className="account-switch-row">
                <span className="account-switch-row-label">Source</span>
                <span>{providerLabel(pendingProviderSwitch.sourceProviderKey)}</span>
              </div>
              <div className="account-switch-row">
                <span className="account-switch-row-label">Run</span>
                <span>{pendingProviderSwitch.sourceRunId}</span>
              </div>
              <div className="account-switch-row">
                <span className="account-switch-row-label">Status</span>
                <span>{pendingProviderSwitch.sourceRunStatus}</span>
              </div>
              <div className="account-switch-row">
                <span className="account-switch-row-label">Target</span>
                <span>{providerLabel(pendingProviderSwitch.targetProviderKey)}</span>
              </div>
              <div className="account-switch-row">
                <span className="account-switch-row-label">Model</span>
                <span>{pendingProviderSwitch.targetModel ?? "Default"}</span>
              </div>
            </div>
            <div className="account-switch-actions">
              <button type="button" className="project-history-confirm-cancel" onClick={cancelProviderSwitch}>
                Cancel
              </button>
              <button type="button" className="project-history-confirm-ok" onClick={() => void confirmProviderSwitch()}>
                Start new chat with {providerLabel(pendingProviderSwitch.targetProviderKey)}
              </button>
            </div>
          </>
        ) : null}
      </div>
    </div>
  );
}

const GATE_RADIO_OPTIONS: { value: string; label: string; description: string }[] = [
  {
    value: "keep-test-fix-code",
    label: "Fix the code",
    description: "Keep the test as the source of truth and repair the code until it passes.",
  },
  {
    value: "suggest-requirement-change",
    label: "Suggest requirement change",
    description: "Have the AI propose a requirement update that would justify this behavior.",
  },
];

function GateBlockModal(): React.ReactElement | null {
  const gateBlock = useStore((s) => s.gateBlock);
  const dismissGateBlock = useStore((s) => s.dismissGateBlock);
  const submitGateDecision = useStore((s) => s.submitGateDecision);
  const stop = useStore((s) => s.stop);
  const [selected, setSelected] = useState<string | null>(null);
  const [customText, setCustomText] = useState("");
  const [submitting, setSubmitting] = useState(false);

  if (!gateBlock) return null;

  const detail = gateBlock.message.replace(/^Flow gate:\s*/i, "");
  const hasOptions = Array.isArray(gateBlock.options) && gateBlock.options.length > 0;

  // r-reg decision card (Task-155)
  if (hasOptions) {
    // Custom text overrides radio selection; one of them must be present to submit.
    const effectiveOption = customText.trim() ? "custom" : selected;
    const canSubmit = Boolean(effectiveOption);
    const secondaryAction = gateBlockSecondaryAction(gateBlock.options);

    const handleSubmit = async () => {
      if (submitting || !canSubmit || !effectiveOption) return;
      setSubmitting(true);
      try {
        await submitGateDecision(
          effectiveOption,
          effectiveOption === "custom" ? customText : undefined,
        );
      } finally {
        setSubmitting(false);
      }
    };

    const tests =
      gateBlock.regressedTests
        ?.filter((t) => t !== "suite_regressed")
        .slice(0, 5)
        .join(", ") ?? "";
    const coarse = gateBlock.regressedTests?.includes("suite_regressed") && !tests;

    return (
      <div
        className="account-switch-overlay"
        role="dialog"
        aria-modal="true"
        aria-label="Regression gate — choose how to proceed"
      >
        <div className="account-switch-modal gate-block-modal gate-decision-card" onClick={(e) => e.stopPropagation()}>
          <p className="gate-block-title">
            <span className="gate-block-icon" aria-hidden="true"><GateIcon size={15} /></span>
            Regression gate — tests broke
          </p>
          {coarse ? (
            <p className="gate-block-detail">The test suite exited with errors (no named tests identified).</p>
          ) : tests ? (
            <p className="gate-block-detail">
              Previously-passing tests are now failing: <strong>{tests}</strong>
              {(gateBlock.regressedTests?.length ?? 0) > 5 ? " …" : ""}
            </p>
          ) : (
            <p className="gate-block-detail">{detail}</p>
          )}
          <p className="gate-block-hint">How would you like to proceed?</p>
          <div className="option-list">
            {GATE_RADIO_OPTIONS.map((opt) => (
              <label
                key={opt.value}
                className={`option option-radio${selected === opt.value ? " selected" : ""}`}
              >
                <input
                  type="radio"
                  name="gate-decision"
                  value={opt.value}
                  checked={selected === opt.value}
                  disabled={submitting}
                  onChange={() => setSelected(opt.value)}
                />
                <span className="option-body">
                  <span className="option-label">{opt.label}</span>
                  <span className="option-desc">{opt.description}</span>
                </span>
              </label>
            ))}
          </div>
          <textarea
            className="gate-custom-textarea"
            placeholder="Custom instruction…"
            rows={3}
            value={customText}
            disabled={submitting}
            onChange={(e) => setCustomText(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey && canSubmit) { e.preventDefault(); void handleSubmit(); } }}
          />
          <div className="account-switch-actions">
            <button
              type="button"
              className="btn btn-ghost"
              disabled={submitting}
              onClick={secondaryAction === "stop-flow" ? () => void stop() : dismissGateBlock}
            >
              {secondaryAction === "stop-flow" ? "Stop flow" : "Dismiss"}
            </button>
            <button
              type="button"
              className="btn btn-primary"
              disabled={submitting || !canSubmit}
              onClick={() => void handleSubmit()}
            >
              Submit
            </button>
          </div>
        </div>
      </div>
    );
  }

  // Plain gate block (non-r-reg violations)
  return (
    <div
      className="account-switch-overlay"
      role="dialog"
      aria-modal="true"
      aria-label="Flow gate blocked the step"
      onClick={dismissGateBlock}
    >
      <div className="account-switch-modal gate-block-modal" onClick={(e) => e.stopPropagation()}>
        <p className="gate-block-title">
          <span className="gate-block-icon" aria-hidden="true"><GateIcon size={15} /></span>
          Flow gate blocked this step
        </p>
        <p className="gate-block-detail">{detail}</p>
        <p className="gate-block-hint">
          Previously-passing tests are failing. This is a hard stop — fix the code so the
          tests pass again. Do not edit or delete the tests to make them green.
        </p>
        <div className="account-switch-actions">
          <button type="button" className="project-history-confirm-ok" onClick={dismissGateBlock}>
            Got it
          </button>
        </div>
      </div>
    </div>
  );
}

// BUG-267: opening/restoring history stamps `unavailableReason` on the row/session but never
// told the user why — this modal fires immediately from the same catch path so the failure is
// visible at click time instead of only in a disabled row's tooltip.
function HistoryOpenErrorModal(): React.ReactElement | null {
  const historyOpenError = useStore((s) => s.historyOpenError);
  const dismissHistoryOpenError = useStore((s) => s.dismissHistoryOpenError);

  if (!historyOpenError) return null;

  const provider = historyOpenError.providerKey ? providerLabel(historyOpenError.providerKey) : "This provider";
  const detail =
    historyOpenError.code === "account_unavailable"
      ? `${provider} isn't set up on this machine. This chat was created with a ${provider} account that isn't available here.`
      : `${provider} is available on this machine, but the active account isn't signed in. Sign in to the account this chat was created with, then try again.`;

  return (
    <div
      className="account-switch-overlay"
      role="dialog"
      aria-modal="true"
      aria-label="Can't open this chat"
      onClick={dismissHistoryOpenError}
    >
      <div className="account-switch-modal gate-block-modal" onClick={(e) => e.stopPropagation()}>
        <p className="gate-block-title">
          <span className="gate-block-icon" aria-hidden="true"><WarnIcon size={15} /></span>
          Can&apos;t open this chat
        </p>
        <p className="gate-block-detail">{detail}</p>
        <div className="account-switch-actions">
          <button type="button" className="project-history-confirm-ok" onClick={dismissHistoryOpenError}>
            Got it
          </button>
        </div>
      </div>
    </div>
  );
}

export function ChatWorkspace({
  leftSidebarVisible,
  rightSidebarVisible,
}: ChatWorkspaceProps): React.ReactElement {
  const workspaceMainView = useStore((s) => s.workspaceMainView);
  const chatMode = useStore((s) => s.chatMode);
  const chatStartMode = useStore((s) => s.chatStartMode);
  const runStatus = useStore((s) => s.status);
  const chatBootStatus = useStore((s) => s.chatBoot.status);

  const activeChatSubMode = chatStartMode;
  const chatAreaClass =
    chatMode === "workflow_step_auto"
      ? "chat-area-flow"
      : activeChatSubMode === "task"
        ? "chat-area-task"
        : activeChatSubMode === "bugfix"
          ? "chat-area-bug"
          : "";
  const isRunning = runStatus === "running";
  const defaultWidthsAppliedRef = useRef(false);
  const shellRef = useRef<HTMLDivElement>(null);
  const dragStateRef = useRef<{
    side: "left" | "right";
    startX: number;
    startWidth: number;
  } | null>(null);
  const [leftSidebarWidth, setLeftSidebarWidth] = useState(320);
  const [rightSidebarWidth, setRightSidebarWidth] = useState(320);

  useEffect(() => {
    if (defaultWidthsAppliedRef.current) return;
    const shell = shellRef.current;
    if (!shell) return;

    const visibleSidebarCount = Number(leftSidebarVisible) + Number(rightSidebarVisible);
    if (visibleSidebarCount === 0) return;

    const resizerWidth = visibleSidebarCount * 8;
    const availableWidth = shell.clientWidth - resizerWidth;
    const targetWidth = Math.round((availableWidth * 1.7) / 8.4);
    const leftDefault = Math.min(380, Math.max(220, targetWidth));
    const rightDefault = Math.min(480, Math.max(300, targetWidth));

    setLeftSidebarWidth(leftDefault);
    setRightSidebarWidth(rightDefault);
    defaultWidthsAppliedRef.current = true;
  }, [leftSidebarVisible, rightSidebarVisible]);

  useEffect(() => {
    const onPointerMove = (event: PointerEvent) => {
      const dragState = dragStateRef.current;
      if (!dragState) return;
      const shell = shellRef.current;
      if (!shell) return;
      const min = dragState.side === "left" ? 220 : 300;
      const max = dragState.side === "left" ? 380 : 480;
      const delta = dragState.side === "left" ? event.clientX - dragState.startX : dragState.startX - event.clientX;
      const next = Math.min(max, Math.max(min, dragState.startWidth + delta));
      if (dragState.side === "left") {
        setLeftSidebarWidth(next);
      } else {
        setRightSidebarWidth(next);
      }
    };

    const stopDragging = () => {
      dragStateRef.current = null;
      document.body.classList.remove("is-resizing-sidebar");
    };

    window.addEventListener("pointermove", onPointerMove);
    window.addEventListener("pointerup", stopDragging);
    window.addEventListener("pointercancel", stopDragging);
    return () => {
      window.removeEventListener("pointermove", onPointerMove);
      window.removeEventListener("pointerup", stopDragging);
      window.removeEventListener("pointercancel", stopDragging);
    };
  }, []);

  const beginResize = (side: "left" | "right") => (event: ReactPointerEvent<HTMLButtonElement>) => {
    const shell = shellRef.current;
    if (!shell) return;
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    dragStateRef.current = {
      side,
      startX: event.clientX,
      startWidth: side === "left" ? leftSidebarWidth : rightSidebarWidth,
    };
    document.body.classList.add("is-resizing-sidebar");
  };

  const workspaceStyle = {
    "--left-sidebar-width": leftSidebarVisible ? `${leftSidebarWidth}px` : "0px",
    "--right-sidebar-width": rightSidebarVisible ? `${rightSidebarWidth}px` : "0px",
  } as CSSProperties;

  return (
    <div ref={shellRef} className={`app-body workspace-shell ${leftSidebarVisible ? "left-visible" : "left-hidden"} ${rightSidebarVisible ? "right-visible" : "right-hidden"}`} style={workspaceStyle}>
      <ProviderSwitchModal />
      <AccountSwitchModal />
      <GrokYoloPostureModal />
      <GateBlockModal />
      <HistoryOpenErrorModal />
      {leftSidebarVisible && (
        <>
          <aside className="sidebar sidebar-left">
            <Navigator />
            <div className="sidebar-bottom">
              <SystemControls />
              <ScenarioSwitcher />
            </div>
          </aside>
          <button
            type="button"
            className="sidebar-resizer sidebar-resizer-left"
            aria-label="Resize left sidebar"
            onPointerDown={beginResize("left")}
          />
        </>
      )}

      <main className={`main workspace-main${chatAreaClass ? ` ${chatAreaClass}` : ""}${isRunning && chatAreaClass ? " chat-area-running" : ""}`}>
        {workspaceMainView === "board" ? (
          <OrchestrationBoard />
        ) : (
          <>
            <Timeline />
            <DispatchAttentionCard />
            <FlowAwaitingUserCard />
            <ChatInput />
          </>
        )}
        <TerminalPanel />
        {/* Boot gate covers the workspace main column (chat and board alike) —
            header tabs, sidebars, and the terminal dock stay interactive while
            workspace data loads. */}
        {chatBootStatus !== "ready" && <ChatBootOverlay />}
      </main>

      <SpectatorPane />

      <FlowTimelineSidebar />

      {rightSidebarVisible && (
        <>
          <button
            type="button"
            className="sidebar-resizer sidebar-resizer-right"
            aria-label="Resize right sidebar"
            onPointerDown={beginResize("right")}
          />
          <aside className="sidebar sidebar-right">
            <div className="right-sidebar-stack">
              <LSPStatusNotice />
              <WorkflowControlPanel />
              <ChatPosturePanel />
              <AgentsPanel />
              <ProviderAccountsPanel />
            </div>
          </aside>
        </>
      )}
    </div>
  );
}
