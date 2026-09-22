import { useEffect, useMemo, useState } from "react";
import type { Project, ProjectWorkspaceBinding } from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { LIBRETRANSLATE_URL } from "@/config";
import { formatTimestamp, toErrorMessage } from "@/components/settings/settingsHelpers";
import {
  dispatchScaffold,
  engineTone,
  fetchApprovalAllowlist,
  fetchGlobalEngineToolingStatus,
  fetchProjectEngineStatus,
  fetchScaffoldStatus,
  initProjectEngine,
  installLibreTranslateTool,
  removeApprovalAllowRule,
  saveProjectEngineGateMode,
  summarizeProjectEngineInit,
  type GlobalEngineToolingStatus,
  type LibreTranslateInstallResult,
  type ProjectEngineStatus,
  type ScaffoldStatusResult,
} from "@/components/settings/projectEngine";
import { ScaffoldActivity } from "@/components/settings/ScaffoldActivity";

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
  const libreTranslatePort = LIBRETRANSLATE_URL.split(":").at(-1) ?? "5001";
  const [loading, setLoading] = useState(true);
  const [toolingBusy, setToolingBusy] = useState(false);
  const [projectBusyAction, setProjectBusyAction] = useState<"refresh" | "init" | "scaffold" | null>(null);
  const [scaffoldStatus, setScaffoldStatus] = useState<ScaffoldStatusResult | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [entries, setEntries] = useState<EngineProjectEntry[]>([]);
  const [selectedProjectId, setSelectedProjectId] = useState("");
  const [selectedBindingPath, setSelectedBindingPath] = useState("");
  const [toolingStatus, setToolingStatus] = useState<GlobalEngineToolingStatus | null>(null);
  const [projectStatus, setProjectStatus] = useState<ProjectEngineStatus | null>(null);
  const [gateMode, setGateMode] = useState<string>("enforce");
  const [gateModeLocal, setGateModeLocal] = useState<string>("enforce");
  const [gateModeSaving, setGateModeSaving] = useState(false);
  // BUG-246: persisted "don't ask again" shell-approval rules for this project.
  const [approvalAllowlist, setApprovalAllowlist] = useState<string[]>([]);
  const [allowlistRemoving, setAllowlistRemoving] = useState<string | null>(null);
  const [libreInstallBusy, setLibreInstallBusy] = useState(false);
  const [libreInstallResult, setLibreInstallResult] = useState<LibreTranslateInstallResult | null>(null);

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
      setApprovalAllowlist([]);
      setScaffoldStatus(null);
      return;
    }
    let active = true;
    // CA-917: load scaffold capability alongside engine status so the manual
    // "Run AI Scaffold" affordance reflects the selected project.
    void fetchScaffoldStatus(selectedProjectId, selectedBindingPath, selectedEntry?.project.platform)
      .then((s) => { if (active) setScaffoldStatus(s); })
      .catch(() => { if (active) setScaffoldStatus(null); });
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
      try {
        const allow = await fetchApprovalAllowlist(selectedProjectId, selectedBindingPath);
        if (active) {
          setApprovalAllowlist(allow);
        }
      } catch {
        // Non-fatal: the allowlist is best-effort; leave whatever was shown.
        if (active) {
          setApprovalAllowlist([]);
        }
      }
    })();
    return () => {
      active = false;
    };
  }, [selectedBindingPath, selectedProjectId]);

  const removeAllowRule = async (rule: string) => {
    if (!selectedProjectId || !selectedBindingPath) return;
    setAllowlistRemoving(rule);
    setMessage(null);
    try {
      const updated = await removeApprovalAllowRule(selectedProjectId, selectedBindingPath, rule);
      setApprovalAllowlist(updated);
      setMessage(`Removed auto-approve rule "${rule}".`);
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to remove the auto-approve rule."));
    } finally {
      setAllowlistRemoving(null);
    }
  };

  const refreshTooling = async () => {
    setMessage(null);
    try {
      await loadToolingStatus();
      setMessage("Global tooling status refreshed.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to refresh global tooling status."));
    }
  };

  const installLibreTranslate = async () => {
    setLibreInstallBusy(true);
    setLibreInstallResult(null);
    const ac = new AbortController();
    const timer = window.setTimeout(() => ac.abort(), 15 * 60 * 1000);
    try {
      const result = await installLibreTranslateTool(ac.signal);
      setLibreInstallResult(result);
      if (result.success) {
        setToolingStatus({ tooling: result.tooling });
      }
    } catch (error) {
      setLibreInstallResult({
        success: false,
        output: "",
        error: toErrorMessage(error, "Install failed."),
        tooling: [],
      });
    } finally {
      window.clearTimeout(timer);
      setLibreInstallBusy(false);
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
    // CA-917: capability check for the manual "Run AI Scaffold" affordance —
    // best-effort so a status failure never blocks the engine panel.
    fetchScaffoldStatus(selectedProjectId, selectedBindingPath, selectedEntry?.project.platform)
      .then(setScaffoldStatus)
      .catch(() => setScaffoldStatus(null));
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

  // CA-917: TUI `/init` parity — Desktop can dispatch the AI scaffold turn
  // manually. The POST blocks for the whole turn; the ScaffoldActivity card
  // renders live progress via the CA-916 feed while it runs.
  const runScaffold = async () => {
    if (!selectedProjectId || !selectedBindingPath) return;
    setProjectBusyAction("scaffold");
    setMessage(null);
    const alreadyDone = scaffoldStatus?.scaffoldStatus?.status === "done";
    try {
      const result = await dispatchScaffold(selectedProjectId, selectedBindingPath, {
        platform: selectedEntry?.project.platform,
        modelName: selectedEntry?.project.defaultModel ?? undefined,
        force: alreadyDone,
      });
      setMessage(result.message ?? `scaffold: ${result.status}`);
      fetchScaffoldStatus(selectedProjectId, selectedBindingPath, selectedEntry?.project.platform)
        .then(setScaffoldStatus)
        .catch(() => {});
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to run AI scaffold."));
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
            <h3>Translation Engine (LibreTranslate)</h3>
            <p>
              Free self-hosted translation — no API key, no credit card. Runs on{" "}
              <code>{LIBRETRANSLATE_URL}</code> after install.
            </p>
          </div>
        </div>

        {/* Python + LibreTranslate status rows */}
        <div className="settings-validation">
          {(["python", "libretranslate"] as const).map((name) => {
            const tool = (toolingStatus?.tooling ?? []).find((t) => t.tool === name);
            const status = tool?.status ?? "unknown";
            return (
              <div className={`validation-row ${engineTone(status)}`} key={name}>
                <span>{name}</span>
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <span>
                    {status}
                    {tool?.version ? ` / ${tool.version}` : ""}
                    {tool?.checkedAt ? ` / ${formatTimestamp(tool.checkedAt)}` : ""}
                  </span>
                  {name === "libretranslate" && status !== "ok" && (
                    <button
                      className="secondary-btn"
                      style={{ fontSize: 11, padding: "2px 10px" }}
                      disabled={libreInstallBusy}
                      onClick={() => void installLibreTranslate()}
                      type="button"
                    >
                      {libreInstallBusy ? "Installing…" : "Install"}
                    </button>
                  )}
                  {name === "python" && status !== "ok" && (
                    <span style={{ fontSize: 11, color: "var(--text-dim)" }}>
                      Install Python 3.8+ first
                    </span>
                  )}
                </div>
              </div>
            );
          })}
        </div>

        {libreInstallBusy && (
          <div className="settings-feedback">
            Installing LibreTranslate… this may take several minutes.
          </div>
        )}
        {libreInstallResult && !libreInstallBusy && (
          <div className={`settings-feedback ${libreInstallResult.success ? "" : "error"}`}>
            {libreInstallResult.success
              ? `LibreTranslate installed. Start it with: libretranslate --load-only en,vi --port ${libreTranslatePort}`
              : (libreInstallResult.error ?? "Install failed.")}
          </div>
        )}

        <div className="settings-note" style={{ marginTop: 10, fontSize: 12, color: "var(--text-dim)", lineHeight: 1.5 }}>
          After installing, start the server with:{" "}
          <code style={{ background: "var(--bg-3)", padding: "1px 5px", borderRadius: 4 }}>
            {`libretranslate --load-only en,vi --port ${libreTranslatePort}`}
          </code>
          <br />
          Downloads ~400 MB of language models on first run.
          Then configure the base URL in <strong>Translation Settings</strong>.
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
            {scaffoldStatus?.capable ? (
              <button
                className="secondary-btn"
                disabled={!selectedProjectId || !selectedBindingPath || projectBusyAction !== null}
                onClick={() => void runScaffold()}
                title={scaffoldStatus.verificationCommand ? `Gate: ${scaffoldStatus.verificationCommand}` : undefined}
                type="button"
              >
                {projectBusyAction === "scaffold"
                  ? "Scaffolding…"
                  : scaffoldStatus.scaffoldStatus?.status === "done"
                    ? "Re-run AI Scaffold"
                    : "Run AI Scaffold"}
              </button>
            ) : null}
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
        {/* CA-916: live AI scaffold transcript for the selected project —
            self-hides when the runner reports no scaffold activity. */}
        <ScaffoldActivity projectId={selectedProjectId || null} />
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
      <div className="settings-subpanel">
        <div className="settings-panel-head">
          <div>
            <h3>Auto-approved commands</h3>
            <p>
              Commands you chose “don’t ask again” for during a YOLO-off run. They auto-approve
              without a prompt (matched by executable + subcommand). This list is per project and
              syncs via Drive. Remove a rule to be asked again.
            </p>
          </div>
        </div>
        {!selectedProjectId || !selectedBindingPath ? (
          <div className="settings-empty" style={{ marginTop: 16 }}>
            Select a project binding above to manage auto-approved commands.
          </div>
        ) : approvalAllowlist.length === 0 ? (
          <div className="settings-empty" style={{ marginTop: 16 }}>
            No auto-approve rules yet. They appear here after you tick “don’t ask again” on a command approval.
          </div>
        ) : (
          <div className="settings-validation">
            {approvalAllowlist.map((rule) => (
              <div className="validation-row" key={rule}>
                <span>
                  <code>{rule}</code>
                </span>
                <button
                  className="secondary-btn"
                  disabled={allowlistRemoving !== null}
                  onClick={() => void removeAllowRule(rule)}
                  type="button"
                >
                  {allowlistRemoving === rule ? "Removing..." : "Remove"}
                </button>
              </div>
            ))}
          </div>
        )}
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
