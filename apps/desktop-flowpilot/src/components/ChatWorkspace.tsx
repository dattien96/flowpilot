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
import { FlowTimelineSidebar } from "@/components/FlowTimelineSidebar";
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
  const projects = useStore((s) => s.projects);

  const project = useMemo(() => projects.find((p) => p.id === selectedProjectId), [projects, selectedProjectId]);
  const selectedWorkflow = useMemo(() => workflows.find((w) => w.id === selectedWorkflowId), [workflows, selectedWorkflowId]);
  const selectedStep = useMemo(() => steps.find((step) => step.id === selectedStepId), [steps, selectedStepId]);

  const resolvedModel = useMemo(() => {
    if (launchMode === "workflow" && selectedWorkflowId) {
      return selectedWorkflow?.model || project?.model || "";
    } else if (launchMode === "step" && selectedStepId) {
      return selectedStep?.model || project?.model || "";
    }
    return "";
  }, [launchMode, selectedWorkflow, selectedStep, project]);

  const resolvedProvider = useMemo(() => {
    if (!resolvedModel) return "";
    const m = resolvedModel.toLowerCase().trim();
    if (m.startsWith("gpt-")) return "Codex";
    if (m.startsWith("gemini-") || m.startsWith("auto-gemini-")) return "Gemini";
    if (m.startsWith("claude-")) return "Claude";
    return "Unknown";
  }, [resolvedModel]);

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

          {/* Resolved Main Agent Card */}
          {resolvedModel ? (
            <div className="main-agent-card" style={{ marginTop: "16px", padding: "12px", border: "1px solid var(--border)", borderRadius: "8px", background: "var(--bg-2)" }}>
              <div style={{ fontWeight: 600, fontSize: "11px", color: "var(--text-dim)", textTransform: "uppercase", marginBottom: "8px", letterSpacing: "0.5px" }}>Main Agent</div>
              <div style={{ display: "flex", flexDirection: "column", gap: "6px" }}>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <span style={{ color: "var(--text-dim)", fontSize: "12px" }}>Model</span>
                  <span style={{ fontFamily: "var(--mono)", fontSize: "12px", color: "var(--text)" }}>{resolvedModel}</span>
                </div>
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <span style={{ color: "var(--text-dim)", fontSize: "12px" }}>Provider</span>
                  <span style={{ fontWeight: 600, fontSize: "12px", color: resolvedProvider === "Codex" ? "var(--codex-brand)" : resolvedProvider === "Claude" ? "var(--claude-brand)" : resolvedProvider === "Gemini" ? "var(--gemini-brand)" : "var(--text)" }}>{resolvedProvider}</span>
                </div>
              </div>
            </div>
          ) : null}
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
  const chatStartMode = useStore((s) => s.chatStartMode);
  const chatSourceDocId = useStore((s) => s.chatSourceDocId);
  const runStatus = useStore((s) => s.status);
  const runId = useStore((s) => s.runId);
  const setChatStartMode = useStore((s) => s.setChatStartMode);
  const setChatSourceDocId = useStore((s) => s.setChatSourceDocId);
  const flowRef = useStore((s) => s.flowRef);
  const setFlowRef = useStore((s) => s.setFlowRef);
  const builtinOrchestrationOptions = useStore((s) => s.builtinOrchestrationOptions);

  if (chatMode !== "normal_chat") return null;

  // BUG-NOTE-CP42 #29: the runner only ever honors changeType/subMode/flowRef
  // on the very first turn (turnCount==0), and the client itself already
  // knows this — sendMessage's own isFirstChatTurn check
  // (chatMode === "normal_chat" && !runId) permanently stops sending these
  // fields the moment a runId exists, which happens on the first send and
  // never resets for the life of this chat. But this picker was only
  // disabled while a turn was actively in flight (isRunning), so as soon as
  // turn 1 completed it became clickable again — letting the user "change"
  // a setting that the client had already permanently stopped transmitting.
  // Lock it once the chat has actually started, not just while running.
  const chatStarted = Boolean(runId);
  const isRunning = runStatus === "running" || chatStarted;

  return (
    <section className="workflow-rail workflow-rail-right chat-start-mode-panel">
      <div className="project-rail-head">
        <div>
          <label>Chat Intent</label>
          <p>
            {chatStarted
              ? "Locked after the first message — start a new chat to change the intent."
              : "Select the intent type for this chat. Disabled while the AI is running."}
          </p>
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
            className={`tab chat-start-mode-tab is-${item.mode} ${chatStartMode === item.mode ? "active" : ""}`}
            disabled={isRunning}
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
            disabled={isRunning}
            placeholder={chatStartMode === "task" ? "Task-NNN (optional)" : "BUG-NNN (optional)"}
            onChange={(event) => setChatSourceDocId(event.target.value)}
          />
          <p className="chat-start-mode-hint">
            If set, the runner names the tracked document directly in the injected first-turn guidance.
          </p>
        </div>
      ) : null}
      {chatStartMode === "bugfix" && builtinOrchestrationOptions.length > 0 ? (
        <div className="nav-group chat-builtin-orchestration">
          <label>Built-in orchestration</label>
          <select
            value={flowRef ?? ""}
            disabled={isRunning}
            onChange={(event) => setFlowRef(event.target.value || undefined)}
          >
            <option value="">None</option>
            {builtinOrchestrationOptions.map((opt) => (
              <option key={opt.flowRef} value={opt.flowRef}>
                {opt.label}
              </option>
            ))}
          </select>
          <p className="chat-start-mode-hint">
            {flowRef
              ? (builtinOrchestrationOptions.find((opt) => opt.flowRef === flowRef)?.description ?? "")
              : "Optional. Runs a built-in review-until-clean loop instead of plain bug chat."}
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
            <span className="gate-block-icon" aria-hidden="true">⛔</span>
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
            <button type="button" className="btn btn-ghost" disabled={submitting} onClick={dismissGateBlock}>
              Dismiss
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
  const runStatus = useStore((s) => s.status);

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

      <main className={`main workspace-main${chatAreaClass ? ` ${chatAreaClass}` : ""}${isRunning && chatAreaClass ? " chat-area-running" : ""}`}>
        {workspaceMainView === "board" ? (
          <OrchestrationBoard />
        ) : (
          <>
            <Timeline />
            <ChatInput />
          </>
        )}
      </main>

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
