import { useEffect } from "react";
import { useStore } from "@/state/store";

// Project / workflow / step selector (the navigator).
export function Navigator(): React.ReactElement {
  const {
    projects,
    workflows,
    steps,
    selectedProjectId,
    selectedWorkflowId,
    selectedStepId,
    loadProjects,
    selectProject,
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
        <label>Workflow</label>
        <select
          value={selectedWorkflowId ?? ""}
          disabled={!selectedProjectId}
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

      <div className="nav-group">
        <label>Step</label>
        <select
          value={selectedStepId ?? ""}
          disabled={!selectedWorkflowId}
          onChange={(e) => selectStep(e.target.value)}
        >
          <option value="" disabled>
            Select a step…
          </option>
          {steps.map((s) => (
            <option key={s.id} value={s.id}>
              {s.order}. {s.name}
              {s.defaultSkill ? ` · /${s.defaultSkill}` : ""}
            </option>
          ))}
        </select>
      </div>
    </div>
  );
}
