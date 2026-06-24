import { useEffect, useMemo, useState } from "react";
import type { Project, ProjectWorkspaceBinding } from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { formatTimestamp, toErrorMessage } from "@/components/settings/settingsHelpers";
import {
  engineTone,
  fetchGlobalEngineToolingStatus,
  fetchProjectEngineStatus,
  initProjectEngine,
  saveProjectEngineGateMode,
  summarizeProjectEngineInit,
  type GlobalEngineToolingStatus,
  type ProjectEngineStatus,
} from "@/components/settings/projectEngine";

interface EngineProjectEntry {
  project: Project;
  bindings: ProjectWorkspaceBinding[];
}

function firstProjectIdWithBinding(entries: EngineProjectEntry[]): string {
  return entries.find((entry) => entry.bindings.length > 0)?.project.id ?? entries[0]?.project.id ?? "";
}

function firstBindingPath(bindings: ProjectWorkspaceBinding[]): string {
  return bindings[0]?.localPath?.trim() ?? "";
}

function selectedProjectEntry(
  entries: EngineProjectEntry[],
  selectedProjectId: string,
): EngineProjectEntry | null {
  return entries.find((entry) => entry.project.id === selectedProjectId) ?? null;
}

export function EngineSettings(): React.ReactElement {
  const [loading, setLoading] = useState(true);
  const [toolingBusy, setToolingBusy] = useState(false);
  const [projectBusyAction, setProjectBusyAction] = useState<"refresh" | "init" | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [entries, setEntries] = useState<EngineProjectEntry[]>([]);
  const [selectedProjectId, setSelectedProjectId] = useState("");
  const [selectedBindingPath, setSelectedBindingPath] = useState("");
  const [toolingStatus, setToolingStatus] = useState<GlobalEngineToolingStatus | null>(null);
  const [projectStatus, setProjectStatus] = useState<ProjectEngineStatus | null>(null);
  const [gateMode, setGateMode] = useState<string>("enforce");
  const [gateModeLocal, setGateModeLocal] = useState<string>("enforce");
  const [gateModeSaving, setGateModeSaving] = useState(false);

  const selectedEntry = useMemo(
    () => selectedProjectEntry(entries, selectedProjectId),
    [entries, selectedProjectId],
  );

  const loadProjects = async () => {
    const admin = await getAdminUseCases();
    const projects = await admin.projects.listProjects();
    const bindingsByProject = await Promise.all(
      projects.map(async (project) => ({
        project,
        bindings: await admin.projects.listBindings(project.id),
      })),
    );
    setEntries(bindingsByProject);
    const nextProjectId = selectedProjectId && bindingsByProject.some((entry) => entry.project.id === selectedProjectId)
      ? selectedProjectId
      : firstProjectIdWithBinding(bindingsByProject);
    setSelectedProjectId(nextProjectId);
    const nextEntry = selectedProjectEntry(bindingsByProject, nextProjectId);
    setSelectedBindingPath(nextEntry ? firstBindingPath(nextEntry.bindings) : "");
  };

  const loadToolingStatus = async () => {
    setToolingBusy(true);
    try {
      setToolingStatus(await fetchGlobalEngineToolingStatus());
    } finally {
      setToolingBusy(false);
    }
  };

  useEffect(() => {
    void (async () => {
      setLoading(true);
      setMessage(null);
      try {
        await Promise.all([loadProjects(), loadToolingStatus()]);
      } catch (error) {
        setMessage(toErrorMessage(error, "Unable to load engine settings."));
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  useEffect(() => {
    const entry = selectedEntry;
    if (!entry) {
      setSelectedBindingPath("");
      setProjectStatus(null);
      return;
    }
    if (!entry.bindings.some((binding) => binding.localPath.trim() === selectedBindingPath)) {
      setSelectedBindingPath(firstBindingPath(entry.bindings));
    }
  }, [selectedBindingPath, selectedEntry]);

  useEffect(() => {
    if (!selectedProjectId || !selectedBindingPath) {
      setProjectStatus(null);
      return;
    }
    let active = true;
    void (async () => {
      try {
        const status = await fetchProjectEngineStatus(selectedProjectId, selectedBindingPath, selectedEntry?.project.platform);
        if (active) {
          setProjectStatus(status);
          setGateMode(status.gateMode);
          setGateModeLocal(status.gateMode);
        }
      } catch (error) {
        if (active) {
          setProjectStatus(null);
          setMessage(toErrorMessage(error, "Unable to load project engine status."));
        }
      }
    })();
    return () => {
      active = false;
    };
  }, [selectedBindingPath, selectedProjectId]);

  const refreshTooling = async () => {
    setMessage(null);
    try {
      await loadToolingStatus();
      setMessage("Global tooling status refreshed.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to refresh global tooling status."));
    }
  };

  const refreshProjectStatus = async () => {
    if (!selectedProjectId || !selectedBindingPath) return;
    setProjectBusyAction("refresh");
    setMessage(null);
    try {
      const status = await fetchProjectEngineStatus(selectedProjectId, selectedBindingPath, selectedEntry?.project.platform);
      setProjectStatus(status);
      setGateMode(status.gateMode);
      setGateModeLocal(status.gateMode);
      setMessage("Project engine status refreshed.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to refresh project engine status."));
    } finally {
      setProjectBusyAction(null);
    }
  };

  const initSelectedProject = async () => {
    if (!selectedProjectId || !selectedBindingPath) return;
    setProjectBusyAction("init");
    setMessage(null);
    try {
      const status = await initProjectEngine(selectedProjectId, selectedBindingPath, "manual", selectedEntry?.project.platform);
      setProjectStatus(status);
      setMessage(summarizeProjectEngineInit(status.lastInit));
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to initialize the selected project engine."));
    } finally {
      setProjectBusyAction(null);
    }
  };

  const saveGateMode = async () => {
    if (!selectedProjectId || !selectedBindingPath) return;
    setGateModeSaving(true);
    setMessage(null);
    try {
      const saved = await saveProjectEngineGateMode(selectedProjectId, selectedBindingPath, gateModeLocal);
      setGateMode(saved);
      setGateModeLocal(saved);
      setMessage(`Flow gate mode set to "${saved}".`);
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to save gate mode."));
    } finally {
      setGateModeSaving(false);
    }
  };

  return (
    <section className="settings-panel">
      <div className="settings-panel-head">
        <div>
          <div className="settings-eyebrow">Engine</div>
          <h2>Engine Setup</h2>
          <p>
            Tooling is machine-global. Skill pack, capability, and initialization state are
            shown for the selected project binding.
          </p>
        </div>
      </div>

      {message ? <div className={`settings-feedback ${message.toLowerCase().includes("unable") ? "error" : ""}`}>{message}</div> : null}
      {loading ? <div className="settings-feedback">Loading engine settings...</div> : null}

      <div className="settings-subpanel">
        <div className="settings-panel-head">
          <div>
            <h3>Global Tooling</h3>
            <p>These checks apply to the whole machine, not to a single project.</p>
          </div>
          <div className="settings-actions" style={{ marginTop: 0 }}>
            <button className="secondary-btn" disabled={toolingBusy} onClick={() => void refreshTooling()} type="button">
              {toolingBusy ? "Refreshing..." : "Refresh Tooling"}
            </button>
          </div>
        </div>

        <div className="settings-validation">
          {(toolingStatus?.tooling ?? []).map((tool) => (
            <div className={`validation-row ${engineTone(tool.status)}`} key={tool.tool}>
              <span>{tool.tool}</span>
              <span>{tool.status}{tool.version ? ` / ${tool.version}` : ""} / {formatTimestamp(tool.checkedAt)}</span>
            </div>
          ))}
          {!toolingStatus?.tooling?.length && !loading ? (
            <div className="settings-empty">No global tooling status available yet.</div>
          ) : null}
        </div>
      </div>

      <div className="settings-subpanel">
        <div className="settings-panel-head">
          <div>
            <h3>Project Skill Pack</h3>
            <p>Select a project binding to inspect capability, skill-pack state, and last init.</p>
          </div>
          <div className="settings-actions" style={{ marginTop: 0 }}>
            <button
              className="secondary-btn"
              disabled={!selectedProjectId || !selectedBindingPath || projectBusyAction !== null}
              onClick={() => void refreshProjectStatus()}
              type="button"
            >
              {projectBusyAction === "refresh" ? "Refreshing..." : "Refresh Project"}
            </button>
            <button
              className="secondary-btn"
              disabled={!selectedProjectId || !selectedBindingPath || projectBusyAction !== null}
              onClick={() => void initSelectedProject()}
              type="button"
            >
              {projectBusyAction === "init" ? "Running..." : "Initialize / Re-sync Project"}
            </button>
          </div>
        </div>

        <div className="settings-grid">
          <label className="settings-field">
            <span>Project</span>
            <select
              value={selectedProjectId}
              onChange={(event) => setSelectedProjectId(event.target.value)}
            >
              {entries.length === 0 ? <option value="">No projects</option> : null}
              {entries.map((entry) => (
                <option key={entry.project.id} value={entry.project.id}>
                  {entry.project.name}
                </option>
              ))}
            </select>
          </label>

          <label className="settings-field">
            <span>Binding</span>
            <select
              disabled={!selectedEntry || selectedEntry.bindings.length === 0}
              value={selectedBindingPath}
              onChange={(event) => setSelectedBindingPath(event.target.value)}
            >
              {selectedEntry?.bindings.length ? null : <option value="">No binding</option>}
              {selectedEntry?.bindings.map((binding) => (
                <option key={binding.id} value={binding.localPath}>
                  {binding.label || binding.localPath}
                </option>
              ))}
            </select>
          </label>
        </div>

        {!selectedEntry ? (
          <div className="settings-empty" style={{ marginTop: 16 }}>Create a project first to manage its skill pack.</div>
        ) : !selectedBindingPath ? (
          <div className="settings-empty" style={{ marginTop: 16 }}>
            The selected project has no saved directory binding yet.
          </div>
        ) : projectStatus ? (
          <div className="project-inline-list" style={{ marginTop: 16 }}>
            <div className="project-inline-card">
              <strong>Binding Summary</strong>
              <div className="project-inline-row"><span>Project</span><span>{selectedEntry.project.name}</span></div>
              <div className="project-inline-row"><span>Working directory</span><span>{projectStatus.workingDirectory}</span></div>
              <div className="project-inline-row"><span>Initialized</span><span>{projectStatus.initialized ? "yes" : "no"}</span></div>
              {projectStatus.warnings?.length ? (
                <div className="settings-feedback" style={{ marginTop: 12 }}>
                  {projectStatus.warnings.join(" ")}
                </div>
              ) : null}
            </div>

            <div className="project-inline-card">
              <strong>Capability</strong>
              <div className="project-inline-row"><span>Structure tier</span><span>{projectStatus.capability.structureTier}</span></div>
              <div className="project-inline-row"><span>Decision tier</span><span>{projectStatus.capability.decisionTier}</span></div>
              <div className="project-inline-row"><span>Tests detected</span><span>{projectStatus.capability.hasTests ? "yes" : "no"}</span></div>
              <div className="project-inline-row"><span>Specs detected</span><span>{projectStatus.capability.hasSpecs ? "yes" : "no"}</span></div>
              <div className="project-inline-row"><span>Languages</span><span>{projectStatus.capability.languages.join(", ") || "none detected"}</span></div>
            </div>

            <div className="project-inline-card">
              <strong>Skill Pack</strong>
              <div className="project-inline-row"><span>Pack version</span><span>{projectStatus.skillPack.packVersion}</span></div>
              <div className="project-inline-row"><span>Installed</span><span>{projectStatus.skillPack.installed ? "yes" : "no"}</span></div>
              <div className="project-inline-row"><span>Current</span><span>{projectStatus.skillPack.current ? "yes" : "no"}</span></div>
              <div className="settings-list" style={{ marginTop: 12 }}>
                {projectStatus.skillPack.skills.map((skill) => (
                  <div className="settings-list-item static" key={skill.name}>
                    <div>
                      <strong>{skill.name}</strong>
                      <span>
                        {skill.providers.map((provider) => `${provider.provider}:${provider.present ? (provider.current ? "current" : "stale") : "missing"}`).join(" · ")}
                      </span>
                    </div>
                  </div>
                ))}
              </div>
            </div>

            <div className="project-inline-card">
              <strong>Last Init</strong>
              <div className="project-inline-row"><span>Summary</span><span>{summarizeProjectEngineInit(projectStatus.lastInit)}</span></div>
              {projectStatus.lastInit ? (
                <>
                  <div className="project-inline-row"><span>Trigger</span><span>{projectStatus.lastInit.trigger}</span></div>
                  <div className="project-inline-row"><span>Status</span><span>{projectStatus.lastInit.status}</span></div>
                  <div className="project-inline-row"><span>Attempted</span><span>{formatTimestamp(projectStatus.lastInit.attemptedAt)}</span></div>
                  <div className="project-inline-row"><span>Completed</span><span>{formatTimestamp(projectStatus.lastInit.completedAt)}</span></div>
                  <div className="settings-validation" style={{ marginTop: 12 }}>
                    {projectStatus.lastInit.steps.map((step) => (
                      <div className={`validation-row ${engineTone(step.outcome)}`} key={`${step.step}:${step.outcome}`}>
                        <span>{step.step}</span>
                        <span>{step.detail || step.errorMessage || step.outcome}</span>
                      </div>
                    ))}
                  </div>
                </>
              ) : (
                <div className="settings-empty" style={{ marginTop: 12 }}>
                  No engine init has been recorded yet.
                </div>
              )}
            </div>
          </div>
        ) : (
          <div className="settings-empty" style={{ marginTop: 16 }}>
            No project engine status is available for the selected binding yet.
          </div>
        )}
      </div>
      <div className="settings-subpanel">
        <div className="settings-panel-head">
          <div>
            <h3>Flow Gate</h3>
            <p>
              The gate runs after every AI turn. Enforce activates reprompt and block actions.
              Warn logs violations without interrupting the step.
            </p>
          </div>
          <div className="settings-actions" style={{ marginTop: 0 }}>
            <button
              className="secondary-btn"
              disabled={!selectedProjectId || !selectedBindingPath || gateModeSaving || gateModeLocal === gateMode}
              onClick={() => void saveGateMode()}
              type="button"
            >
              {gateModeSaving ? "Saving..." : "Save"}
            </button>
          </div>
        </div>

        {selectedProjectId && selectedBindingPath ? (
          <div className="settings-grid" style={{ marginBottom: 20 }}>
            <label className="settings-field">
              <span>Gate mode</span>
              <select
                value={gateModeLocal}
                onChange={(event) => setGateModeLocal(event.target.value)}
              >
                <option value="enforce">Enforce (default) — reprompt + block on violations</option>
                <option value="warn">Warn only — log violations, never block</option>
              </select>
            </label>
          </div>
        ) : (
          <div className="settings-empty" style={{ marginTop: 16 }}>
            Select a project binding above to configure the flow gate.
          </div>
        )}

        <div className="settings-validation">
          {GATE_RULES.map((rule) => (
            <div className={`validation-row ${rule.alwaysEnforced ? "passed" : gateModeLocal === "enforce" ? "passed" : "warn"}`} key={rule.id}>
              <span>
                <strong>{rule.id}</strong>
                {"  "}
                {rule.description}
              </span>
              <span>
                {rule.alwaysEnforced
                  ? "always blocks"
                  : gateModeLocal === "enforce"
                    ? rule.enforceAction
                    : "warn only"}
              </span>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

const GATE_RULES: Array<{
  id: string;
  description: string;
  enforceAction: string;
  alwaysEnforced: boolean;
}> = [
  {
    id: "r-ca",
    description: "Code changed without a change-audit note",
    enforceAction: "reprompt",
    alwaysEnforced: false,
  },
  {
    id: "r-bug",
    description: "Bug fix without a BugFix doc in requirements/",
    enforceAction: "block",
    alwaysEnforced: false,
  },
  {
    id: "r-tests",
    description: "Task tests failing at step completion",
    enforceAction: "block",
    alwaysEnforced: true,
  },
  {
    id: "r-reg",
    description: "Previously-green test now fails (regression)",
    enforceAction: "block",
    alwaysEnforced: true,
  },
  {
    id: "r-dep",
    description: "Deleted file still referenced by callers",
    enforceAction: "block",
    alwaysEnforced: false,
  },
];
