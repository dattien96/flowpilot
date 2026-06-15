import { useEffect } from "react";
import { useStore } from "@/state/store";
import { ProviderAccountsPanel } from "@/components/ProviderAccountsPanel";
import { filterNavigatorWorkflows } from "@/app/navigatorCatalog";
import type { ProviderKey } from "@/types/contract";
import type { ChatMode } from "@/state/store";

const PROVIDER_OPTIONS: { value: ProviderKey; label: string }[] = [
  { value: "codex", label: "Codex" },
  { value: "claude", label: "Claude" },
  { value: "gemini", label: "Gemini" },
];

const REASONING_OPTIONS = [
  { value: "", label: "Default" },
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
];

// Project / workflow / step selector (the navigator).
export function Navigator(): React.ReactElement {
  const {
    projects,
    workflows,
    steps,
    supportedModels,
    selectedProjectId,
    selectedWorkflowId,
    selectedStepId,
    launchMode,
    chatMode,
    selectedProvider,
    selectedModel,
    reasoningEffort,
    yoloMode,
    loadProjects,
    selectProject,
    setLaunchMode,
    setChatMode,
    selectProvider,
    setSelectedModel,
    setReasoningEffort,
    setYoloMode,
    selectWorkflow,
    selectStep,
  } = useStore();
  const visibleWorkflows = filterNavigatorWorkflows(workflows, selectedProjectId);

  const modelsForProvider = supportedModels.filter(
    (m) => m.isEnabled && (!selectedProvider || m.providerKey === selectedProvider),
  );

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

      {/* Top-level mode: Chat vs Workflow */}
      <div className="nav-group">
        <label>Mode</label>
        <div className="tab-list" role="tablist" aria-label="Chat mode">
          {(["normal_chat", "workflow_step_auto"] as ChatMode[]).map((mode) => (
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
      </div>

      {chatMode === "normal_chat" ? (
        /* ── Chat mode controls ── */
        <>
          <div className="nav-group">
            <label>Provider</label>
            <select
              value={selectedProvider ?? ""}
              onChange={(e) =>
                selectProvider(e.target.value ? (e.target.value as ProviderKey) : undefined)
              }
            >
              <option value="" disabled>
                Select a provider…
              </option>
              {PROVIDER_OPTIONS.map((p) => (
                <option key={p.value} value={p.value}>
                  {p.label}
                </option>
              ))}
            </select>
          </div>

          <div className="nav-group">
            <label>Model</label>
            {modelsForProvider.length > 0 ? (
              <select
                value={selectedModel ?? ""}
                onChange={(e) => setSelectedModel(e.target.value || undefined)}
              >
                <option value="">Default</option>
                {modelsForProvider.map((m) => (
                  <option key={m.id} value={m.modelId}>
                    {m.displayName}
                  </option>
                ))}
              </select>
            ) : (
              <input
                type="text"
                className="nav-text-input"
                placeholder="e.g. claude-sonnet-4-5"
                value={selectedModel ?? ""}
                onChange={(e) => setSelectedModel(e.target.value || undefined)}
              />
            )}
          </div>

          <div className="nav-group">
            <label>Reasoning effort</label>
            <select
              value={reasoningEffort ?? ""}
              onChange={(e) => setReasoningEffort(e.target.value || undefined)}
            >
              {REASONING_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </div>

          <div className="nav-group nav-group-inline">
            <label htmlFor="yolo-toggle">YOLO mode</label>
            <input
              id="yolo-toggle"
              type="checkbox"
              checked={yoloMode}
              onChange={(e) => setYoloMode(e.target.checked)}
            />
            <div className="nav-hint">Skip approval prompts (auto-approve all tool calls).</div>
          </div>
        </>
      ) : (
        /* ── Workflow mode controls ── */
        <>
          <div className="nav-group">
            <label>Provider</label>
            <select
              value={selectedProvider ?? ""}
              onChange={(e) =>
                selectProvider(e.target.value ? (e.target.value as ProviderKey) : undefined)
              }
            >
              <option value="">Auto (from model)</option>
              {PROVIDER_OPTIONS.map((p) => (
                <option key={p.value} value={p.value}>
                  {p.label}
                </option>
              ))}
            </select>
            <div className="nav-hint">
              Auto: workflow/step picks the provider from its model. Choose one to override.
            </div>
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
                {visibleWorkflows.map((w) => (
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
        </>
      )}

      <ProviderAccountsPanel />
    </div>
  );
}
