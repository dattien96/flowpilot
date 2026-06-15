import { useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties, PointerEvent as ReactPointerEvent } from "react";
import { filterNavigatorWorkflows } from "@/app/navigatorCatalog";
import { Navigator } from "@/components/Navigator";
import { ChatInput } from "@/components/ChatInput";
import { Timeline } from "@/components/Timeline";
import { ScenarioSwitcher } from "@/components/ScenarioSwitcher";
import { SystemControls } from "@/components/SystemControls";
import { ProviderAccountsPanel } from "@/components/ProviderAccountsPanel";
import { useStore } from "@/state/store";

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

interface ChatWorkspaceProps {
  leftSidebarVisible: boolean;
  rightSidebarVisible: boolean;
}

export function ChatWorkspace({
  leftSidebarVisible,
  rightSidebarVisible,
}: ChatWorkspaceProps): React.ReactElement {
  const shellRef = useRef<HTMLDivElement>(null);
  const dragStateRef = useRef<{
    side: "left" | "right";
    startX: number;
    startWidth: number;
  } | null>(null);
  const [leftSidebarWidth, setLeftSidebarWidth] = useState(280);
  const [rightSidebarWidth, setRightSidebarWidth] = useState(360);

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

      <main className="main workspace-main">
        <Timeline />
        <ChatInput />
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
              <ProviderAccountsPanel />
            </div>
          </aside>
        </>
      )}
    </div>
  );
}
