import { useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, PointerEvent as ReactPointerEvent } from "react";
import { filterNavigatorWorkflows } from "@/app/navigatorCatalog";
import { Navigator } from "@/components/Navigator";
import { ChatInput } from "@/components/ChatInput";
import { Timeline } from "@/components/Timeline";
import { ScenarioSwitcher } from "@/components/ScenarioSwitcher";
import { SystemControls } from "@/components/SystemControls";
import { ProviderAccountsPanel } from "@/components/ProviderAccountsPanel";
import { AgentsPanel } from "@/components/AgentsPanel";
import { OrchestrationBoard } from "@/components/OrchestrationBoard";
import { useStore, accountLabel, providerLabel, type ChatStartMode } from "@/state/store";

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
                {visibleWorkflows.map((workflow) => (
                  <option key={workflow.id} value={workflow.id}>
                    {workflow.name}
                  </option>
                ))}
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
                {steps.map((step) => (
                  <option key={step.id} value={step.id}>
                    {step.name}
                  </option>
                ))}
              </select>
            </div>
          </div>
        </>
      )}
    </section>
  );
}

function ChatModeIntentIcon({ mode }: { mode: ChatStartMode }): React.ReactElement {
  if (mode === "task") {
    return (
      <svg viewBox="0 0 16 16" aria-hidden="true">
        <rect x="3" y="2.5" width="10" height="11" rx="2" fill="none" stroke="currentColor" strokeWidth="1.4" />
        <path d="M5.3 5.7h5.4M5.3 8h5.4M5.3 10.3h3.2" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
      </svg>
    );
  }
  if (mode === "bugfix") {
    return (
      <svg viewBox="0 0 16 16" aria-hidden="true">
        <path d="M8 3.1c1.8 0 3.2 1.3 3.2 3v2c0 2.3-1.4 4.3-3.2 4.3S4.8 10.4 4.8 8.1v-2c0-1.7 1.4-3 3.2-3z" fill="none" stroke="currentColor" strokeWidth="1.3" />
        <path d="M6.2 2.4 5.2 1.2M9.8 2.4l1-1.2M4.1 6 2.4 5.1M11.9 6l1.7-.9M4 9.4l-1.7.9M12 9.4l1.7.9" fill="none" stroke="currentColor" strokeWidth="1.2" strokeLinecap="round" />
      </svg>
    );
  }
  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <circle cx="8" cy="8" r="4.5" fill="none" stroke="currentColor" strokeWidth="1.4" />
    </svg>
  );
}

function ChatStartIntentPanel(): React.ReactElement | null {
  const chatMode = useStore((s) => s.chatMode);
  const timeline = useStore((s) => s.timeline);
  const chatStartMode = useStore((s) => s.chatStartMode);
  const chatSourceDocId = useStore((s) => s.chatSourceDocId);
  const lastTurnInput = useStore((s) => s.lastTurnInput);
  const setChatStartMode = useStore((s) => s.setChatStartMode);
  const setChatSourceDocId = useStore((s) => s.setChatSourceDocId);

  if (chatMode !== "normal_chat") return null;

  const hasTurns = timeline.length > 0;
  const activeMode = hasTurns ? (lastTurnInput?.changeType ?? "normal") : chatStartMode;
  const activeDocId = hasTurns ? (lastTurnInput?.sourceDocId ?? "") : chatSourceDocId;

  if (hasTurns) {
    if (activeMode === "normal") return null;
    return (
      <section className="workflow-rail workflow-rail-right chat-start-mode-panel">
        <div className="project-rail-head">
          <div>
            <label>Declared Intent</label>
            <p>This chat is locked to the selected flow-gate intent.</p>
          </div>
        </div>
        <div className="chat-start-mode-summary">
          <div className={`chat-start-mode-chip is-${activeMode}`}>
            <span className="chat-start-mode-icon"><ChatModeIntentIcon mode={activeMode} /></span>
            <span>{activeMode === "task" ? "Task" : "Bug"}</span>
          </div>
          {activeDocId ? <div className="chat-start-mode-docid">{activeDocId}</div> : <div className="chat-start-mode-docid muted">No document id declared</div>}
        </div>
      </section>
    );
  }

  return (
    <section className="workflow-rail workflow-rail-right chat-start-mode-panel">
      <div className="project-rail-head">
        <div>
          <label>Chat Intent</label>
          <p>Declare Task or Bug before the first turn so the runner can mark this chat without relying on response text.</p>
        </div>
      </div>
      <div className="tab-list tab-list-three" role="tablist" aria-label="Chat start intent">
        {([
          { mode: "normal" as const, label: "Normal" },
          { mode: "task" as const, label: "Task" },
          { mode: "bugfix" as const, label: "Bug" },
        ]).map((item) => (
          <button
            key={item.mode}
            type="button"
            role="tab"
            aria-selected={chatStartMode === item.mode}
            className={`tab chat-start-mode-tab ${chatStartMode === item.mode ? "active" : ""}`}
            onClick={() => setChatStartMode(item.mode)}
          >
            <span className="chat-start-mode-icon"><ChatModeIntentIcon mode={item.mode} /></span>
            <span>{item.label}</span>
          </button>
        ))}
      </div>
      {chatStartMode !== "normal" ? (
        <div className="nav-group">
          <label>{chatStartMode === "task" ? "Task ID (optional)" : "Bug ID (optional)"}</label>
          <input
            value={chatSourceDocId}
            placeholder={chatStartMode === "task" ? "Task-NNN (optional)" : "BUG-NNN (optional)"}
            onChange={(event) => setChatSourceDocId(event.target.value)}
          />
          <p className="chat-start-mode-hint">
            If set, the runner names the tracked document directly in the injected first-turn guidance.
          </p>
        </div>
      ) : null}
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

function GateBlockModal(): React.ReactElement | null {
  const gateBlock = useStore((s) => s.gateBlock);
  const dismissGateBlock = useStore((s) => s.dismissGateBlock);

  if (!gateBlock) return null;

  // The runner prefixes the message with "Flow gate: "; strip it for the body since
  // the modal title already says "Flow gate".
  const detail = gateBlock.message.replace(/^Flow gate:\s*/i, "");

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
          <span className="gate-block-icon" aria-hidden="true">⛔</span>
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

export function ChatWorkspace({
  leftSidebarVisible,
  rightSidebarVisible,
}: ChatWorkspaceProps): React.ReactElement {
  const workspaceMainView = useStore((s) => s.workspaceMainView);
  const chatMode = useStore((s) => s.chatMode);
  const chatStartMode = useStore((s) => s.chatStartMode);
  const timeline = useStore((s) => s.timeline);
  const lastTurnInput = useStore((s) => s.lastTurnInput);

  const hasTurns = timeline.length > 0;
  const activeChatSubMode = hasTurns ? (lastTurnInput?.changeType ?? "normal") : chatStartMode;
  const chatAreaClass =
    chatMode === "workflow_step_auto"
      ? "chat-area-flow"
      : activeChatSubMode === "task"
        ? "chat-area-task"
        : activeChatSubMode === "bugfix"
          ? "chat-area-bug"
          : "";
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
      <AccountSwitchModal />
      <GateBlockModal />
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

      <main className={`main workspace-main${chatAreaClass ? ` ${chatAreaClass}` : ""}`}>
        {workspaceMainView === "board" ? (
          <OrchestrationBoard />
        ) : (
          <>
            <Timeline />
            <ChatInput />
          </>
        )}
      </main>

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
              <WorkflowControlPanel />
              <ChatStartIntentPanel />
              <AgentsPanel />
              <ProviderAccountsPanel />
            </div>
          </aside>
        </>
      )}
    </div>
  );
}
