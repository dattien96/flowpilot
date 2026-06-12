import { useEffect } from "react";
import { useStore } from "@/state/store";
import { ProviderAccountsPanel } from "@/components/ProviderAccountsPanel";

// Project / workflow / step selector (the navigator).
export function Navigator(): React.ReactElement {
  const {
    projects,
    workflows,
    steps,
    selectedProjectId,
    selectedWorkflowId,
    selectedStepId,
    launchMode,
    loadProjects,
    selectProject,
    setLaunchMode,
    selectWorkflow,
    selectStep,
  } = useStore();

  useEffect(() => {
    void loadProjects();
  }, [loadProjects]);

  return (
    <div className="navigator">
      <div className="nav-group">
        <label>Project</label>
        <select value={selectedProjectId ?? ""} onChange={(e) => void selectProject(e.target.value)}>
          <option value="" disabled>
            Select a project…
          </option>
          {projects.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
        {selectedProjectId && (
          <div className="nav-hint">{projects.find((p) => p.id === selectedProjectId)?.path}</div>
        )}
      </div>

      <div className="nav-group">
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
            {workflows.map((w) => (
              <option key={w.id} value={w.id}>
                {w.name}
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
            {steps.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
                {s.defaultSkill ? ` · /${s.defaultSkill}` : ""}
              </option>
            ))}
          </select>
        </div>
      </div>

      <ProviderAccountsPanel />
    </div>
  );
}
