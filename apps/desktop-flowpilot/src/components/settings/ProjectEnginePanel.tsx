import { useEffect, useState } from "react";
import type { Project } from "@flowpilot/client-core";
import { formatTimestamp, toErrorMessage } from "@/components/settings/settingsHelpers";
import {
  engineTone,
  fetchProjectEngineStatus,
  fetchScaffoldStatus,
  initProjectEngine,
  summarizeProjectEngineInit,
  type ProjectEngineStatus,
  type ScaffoldStatusResult,
} from "@/components/settings/projectEngine";

// CA-1000: per-project engine surface that lives inside the Projects detail
// column (moved out of Settings → Engine). Covers skill-pack status +
// Initialize/Re-sync + the manual AI Scaffold trigger; the global/tooling and
// gate/allowlist settings stay on the Engine page.
export interface ScaffoldLaunchInput {
  projectId: string;
  workingDirectory: string;
  platform?: string;
  modelName?: string;
  force?: boolean;
}

interface ProjectEnginePanelProps {
  project: Project;
  bindings: Array<{ localPath: string; label?: string | null }>;
  /** The host decides what "run" means — Projects launches the chat-streamed
   *  scaffold (Chat view + runScaffoldChat) via this callback. */
  onRunScaffold: (input: ScaffoldLaunchInput) => void;
}

export function ProjectEnginePanel({ project, bindings, onRunScaffold }: ProjectEnginePanelProps): React.ReactElement {
  const usableBindings = bindings.filter((binding) => binding.localPath.trim().length > 0);
  const [selectedBindingPath, setSelectedBindingPath] = useState(usableBindings[0]?.localPath.trim() ?? "");
  const [projectStatus, setProjectStatus] = useState<ProjectEngineStatus | null>(null);
  const [scaffoldStatus, setScaffoldStatus] = useState<ScaffoldStatusResult | null>(null);
  const [busyAction, setBusyAction] = useState<"refresh" | "init" | null>(null);
  const [message, setMessage] = useState<string | null>(null);

  useEffect(() => {
    if (!usableBindings.some((binding) => binding.localPath.trim() === selectedBindingPath)) {
      setSelectedBindingPath(usableBindings[0]?.localPath.trim() ?? "");
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [project.id, bindings]);

  useEffect(() => {
    if (!selectedBindingPath) {
      setProjectStatus(null);
      setScaffoldStatus(null);
      return;
    }
    let active = true;
    void fetchScaffoldStatus(project.id, selectedBindingPath, project.platform)
      .then((status) => { if (active) setScaffoldStatus(status); })
      .catch(() => { if (active) setScaffoldStatus(null); });
    void fetchProjectEngineStatus(project.id, selectedBindingPath, project.platform)
      .then((status) => { if (active) setProjectStatus(status); })
      .catch(() => { if (active) setProjectStatus(null); });
    return () => {
      active = false;
    };
  }, [project.id, project.platform, selectedBindingPath]);

  const refreshProjectStatus = async () => {
    if (!selectedBindingPath) return;
    setBusyAction("refresh");
    setMessage(null);
    try {
      setProjectStatus(await fetchProjectEngineStatus(project.id, selectedBindingPath, project.platform));
      setMessage("Project engine status refreshed.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to refresh project engine status."));
    } finally {
      setBusyAction(null);
    }
    fetchScaffoldStatus(project.id, selectedBindingPath, project.platform)
      .then(setScaffoldStatus)
      .catch(() => setScaffoldStatus(null));
  };

  const initProject = async () => {
    if (!selectedBindingPath) return;
    setBusyAction("init");
    setMessage(null);
    try {
      const status = await initProjectEngine(project.id, selectedBindingPath, "manual", project.platform);
      setProjectStatus(status);
      setMessage(summarizeProjectEngineInit(status.lastInit));
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to initialize the project engine."));
    } finally {
      setBusyAction(null);
    }
  };

  const runScaffold = () => {
    if (!selectedBindingPath) return;
    onRunScaffold({
      projectId: project.id,
      workingDirectory: selectedBindingPath,
      platform: project.platform,
      modelName: project.defaultModel ?? undefined,
      force: scaffoldStatus?.scaffoldStatus?.status === "done",
    });
  };

  return (
    <div>
      <div className="settings-grid">
        <label className="settings-field">
          <span>Binding</span>
          <select
            disabled={usableBindings.length === 0}
            value={selectedBindingPath}
            onChange={(event) => setSelectedBindingPath(event.target.value)}
          >
            {usableBindings.length === 0 ? <option value="">No binding</option> : null}
            {usableBindings.map((binding) => (
              <option key={binding.localPath} value={binding.localPath}>
                {binding.label || binding.localPath}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div className="settings-actions" style={{ marginTop: 4 }}>
        <button
          className="secondary-btn"
          disabled={!selectedBindingPath || busyAction !== null}
          onClick={() => void refreshProjectStatus()}
          type="button"
        >
          {busyAction === "refresh" ? "Refreshing..." : "Refresh"}
        </button>
        <button
          className="secondary-btn"
          disabled={!selectedBindingPath || busyAction !== null}
          onClick={() => void initProject()}
          type="button"
        >
          {busyAction === "init" ? "Running..." : "Initialize / Re-sync"}
        </button>
        {scaffoldStatus?.capable ? (
          <button
            className="secondary-btn"
            disabled={!selectedBindingPath || busyAction !== null}
            onClick={runScaffold}
            title={scaffoldStatus.verificationCommand ? `Gate: ${scaffoldStatus.verificationCommand}` : undefined}
            type="button"
          >
            {scaffoldStatus.scaffoldStatus?.status === "done" ? "Re-run AI Scaffold" : "Run AI Scaffold"}
          </button>
        ) : null}
      </div>

      {message ? <div className={`settings-feedback ${message.toLowerCase().includes("unable") ? "error" : ""}`}>{message}</div> : null}

      {usableBindings.length === 0 ? (
        <div className="settings-empty" style={{ marginTop: 12 }}>
          Save a directory binding for this project to manage its skill pack.
        </div>
      ) : projectStatus ? (
        <div className="project-inline-list" style={{ marginTop: 16 }}>
          <div className="project-inline-card">
            <strong>Binding Summary</strong>
            <div className="project-inline-row"><span>Project</span><span>{project.name}</span></div>
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
        <div className="settings-empty" style={{ marginTop: 12 }}>
          No project engine status is available for the selected binding yet.
        </div>
      )}
    </div>
  );
}
