import { useEffect, useMemo, useState } from "react";
import type { ArtifactRun, LocalRunnerArtifact, LocalRunnerStorageDriver, Project } from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { RUNNER_URL } from "@/config";
import { formatTimestamp, toErrorMessage } from "@/components/settings/settingsHelpers";

type Tab = "generated" | "storage";
type GeneratedSubTab = "local" | "remote";

export function ArtifactsSettings(): React.ReactElement {
  const [tab, setTab] = useState<Tab>("generated");
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectId] = useState("");
  const [localArtifacts, setLocalArtifacts] = useState<LocalRunnerArtifact[]>([]);
  const [runs, setRuns] = useState<ArtifactRun[]>([]);
  const [driver, setDriver] = useState<LocalRunnerStorageDriver | null>(null);
  const [storagePreference, setStoragePreference] = useState<"supabase" | "google_drive">("supabase");
  const [busy, setBusy] = useState(false);
  const [syncBusy, setSyncBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [generatedSubTab, setGeneratedSubTab] = useState<GeneratedSubTab>("local");
  const [expandedRuns, setExpandedRuns] = useState<Set<string>>(new Set());

  const refresh = async () => {
    try {
      const admin = await getAdminUseCases();
      const [nextProjects, nextLocal, nextRuns, nextDriver] = await Promise.all([
        admin.projects.listProjects(),
        admin.artifacts.listLocalArtifacts(),
        admin.artifacts.listRuns(),
        admin.artifacts.getStorageDriver(),
      ]);
      setProjects(nextProjects);
      setLocalArtifacts(nextLocal);
      setRuns(nextRuns);
      setDriver(nextDriver);
      const nextProjectId = projectId || nextProjects[0]?.id || "";
      setProjectId(nextProjectId);
      const project = nextProjects.find((item) => item.id === nextProjectId) ?? nextProjects[0] ?? null;
      setStoragePreference(project?.artifactStoragePreference ?? "supabase");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to load artifacts."));
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

  const visibleLocalArtifacts = useMemo(
    () => localArtifacts.filter((item) => !projectId || item.projectId === projectId),
    [localArtifacts, projectId],
  );
  const visibleRuns = useMemo(
    () => runs.filter((item) => !projectId || item.projectId === projectId),
    [runs, projectId],
  );

  // Generated tab: local = runner-local artifacts + supabase runs still local_only
  const localOnlyRuns = useMemo(() => visibleRuns.filter((r) => r.syncStatus === "local_only"), [visibleRuns]);
  const localCount = visibleLocalArtifacts.length + localOnlyRuns.length;

  // Remote = runs with a terminal or in-progress remote sync state
  const remoteRuns = useMemo(() => visibleRuns.filter((r) => r.syncStatus !== "local_only"), [visibleRuns]);

  // Group remote runs by project → provider → workflowRunId
  const remoteGroups = useMemo(() => {
    const groups: Record<string, Record<string, Record<string, ArtifactRun[]>>> = {};
    for (const run of remoteRuns) {
      const projectName = projects.find((p) => p.id === run.projectId)?.name ?? run.projectId ?? "Unknown";
      const provider = (run.storageProvider ?? "unknown").toUpperCase();
      const runId = run.workflowRunId;
      groups[projectName] ??= {};
      groups[projectName][provider] ??= {};
      groups[projectName][provider][runId] ??= [];
      groups[projectName][provider][runId].push(run);
    }
    return groups;
  }, [remoteRuns, projects]);

  const toggleRun = (runId: string) => {
    setExpandedRuns((current) => {
      const next = new Set(current);
      if (next.has(runId)) next.delete(runId);
      else next.add(runId);
      return next;
    });
  };

  const syncArtifacts = async () => {
    setSyncBusy(true);
    setMessage(null);
    try {
      for (const item of visibleLocalArtifacts) {
        await fetch(`${RUNNER_URL}/artifacts/${item.artifactId}/sync`, {
          method: "POST",
          cache: "no-store",
        });
      }
      await refresh();
      setMessage(
        visibleLocalArtifacts.length > 0
          ? `Synced ${visibleLocalArtifacts.length} artifact(s).`
          : "No local artifacts to sync.",
      );
    } catch (error) {
      setMessage(toErrorMessage(error, "Sync failed."));
    } finally {
      setSyncBusy(false);
    }
  };

  const saveStorage = async () => {
    if (!driver) return;
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      if (storagePreference === "google_drive" && driver.enabled && (!driver.remoteRootPath.trim() || !driver.remoteFolderName.trim())) {
        throw new Error("Remote root path and folder name are required before enabling sync.");
      }
      if (!projectId) {
        throw new Error("Select a project before saving storage settings.");
      }
      await admin.projects.updateProject(projectId, { artifactStoragePreference: storagePreference });
      await admin.artifacts.saveStorageDriver(driver);
      await refresh();
      setMessage("Storage settings saved.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to save storage settings."));
    } finally {
      setBusy(false);
    }
  };

  const selectedProject = projects.find((p) => p.id === projectId) ?? null;

  return (
    <section className="settings-panel">
      <div className="settings-panel-head">
        <div>
          <div className="settings-eyebrow">Artifacts</div>
          <h2>Artifacts</h2>
          <p>Browse generated artifacts, configure storage sync, and manage artifact definitions.</p>
        </div>
        <div className="header-tabs">
          <button className={`header-tab ${tab === "generated" ? "active" : ""}`} onClick={() => setTab("generated")} type="button">Generated</button>
          <button className={`header-tab ${tab === "storage" ? "active" : ""}`} onClick={() => setTab("storage")} type="button">Storage</button>
        </div>
      </div>
      {message ? <div className="settings-feedback">{message}</div> : null}

      {/* ── Generated tab ─────────────────────────────────────── */}
      {tab === "generated" ? (
        <div className="settings-subpanel">
          <div className="settings-actions">
            <select value={projectId} onChange={(e) => setProjectId(e.target.value)}>
              <option value="">All projects</option>
              {projects.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </select>
            <button className="secondary-btn" disabled={syncBusy} onClick={() => void syncArtifacts()} type="button">
              {syncBusy ? "Syncing…" : `Sync artifacts (${visibleLocalArtifacts.length})`}
            </button>
          </div>

          <div className="artifact-subtabs">
            <button
              className={`artifact-subtab ${generatedSubTab === "local" ? "active" : ""}`}
              onClick={() => setGeneratedSubTab("local")}
              type="button"
            >
              Local / Not Synced ({localCount})
            </button>
            <button
              className={`artifact-subtab ${generatedSubTab === "remote" ? "active" : ""}`}
              onClick={() => setGeneratedSubTab("remote")}
              type="button"
            >
              Remote / Sync ({remoteRuns.length})
            </button>
          </div>

          {generatedSubTab === "local" ? (
            <div className="settings-list">
              {visibleLocalArtifacts.map((item) => (
                <div className="settings-list-item static" key={item.artifactId}>
                  <strong>{item.title}</strong>
                  <span>{item.syncStatus} · {formatTimestamp(item.updatedAt)}</span>
                </div>
              ))}
              {localOnlyRuns.map((item) => (
                <div className="settings-list-item static" key={item.id}>
                  <strong>{item.title}</strong>
                  <span>local_only · {formatTimestamp(item.updatedAt)}</span>
                </div>
              ))}
              {localCount === 0 ? <div className="settings-list-empty">No local artifacts.</div> : null}
            </div>
          ) : (
            <div className="settings-list">
              {Object.entries(remoteGroups).map(([projectName, byProvider]) => {
                const totalCount = Object.values(byProvider).flatMap(Object.values).flat().length;
                return (
                  <div className="artifact-project-group" key={projectName}>
                    <div className="artifact-group-header">
                      <span className="artifact-group-name">{projectName}</span>
                      <span className="artifact-group-count">{totalCount}</span>
                    </div>
                    {Object.entries(byProvider).map(([provider, byRun]) => {
                      const providerCount = Object.values(byRun).flat().length;
                      return (
                        <div className="artifact-provider-group" key={provider}>
                          <div className="artifact-provider-header">
                            <span>{provider}</span>
                            <span className="artifact-group-count">{providerCount}</span>
                          </div>
                          {Object.entries(byRun).map(([runId, artifacts]) => {
                            const expanded = expandedRuns.has(runId);
                            const status = artifacts[0]?.syncStatus ?? "";
                            return (
                              <div className="artifact-run-row" key={runId}>
                                <button className="artifact-run-header" onClick={() => toggleRun(runId)} type="button">
                                  <span className="artifact-run-chevron">{expanded ? "▾" : "▸"}</span>
                                  <span className="artifact-run-label">WORKFLOW RUN</span>
                                  <span className="artifact-run-id" title={runId}>{runId.slice(0, 8)}…</span>
                                  <span className={`artifact-status-badge artifact-status-${status}`}>{status.toUpperCase()}</span>
                                  <span className="artifact-run-count">{artifacts.length} ARTIFACT{artifacts.length !== 1 ? "S" : ""}</span>
                                </button>
                                {expanded ? (
                                  <div className="artifact-run-items">
                                    {artifacts.map((a) => (
                                      <div className="settings-list-item static" key={a.id}>
                                        <strong>{a.title}</strong>
                                        <span>{a.remotePath || a.localPath} · {formatTimestamp(a.updatedAt)}</span>
                                      </div>
                                    ))}
                                  </div>
                                ) : null}
                              </div>
                            );
                          })}
                        </div>
                      );
                    })}
                  </div>
                );
              })}
              {remoteRuns.length === 0 ? <div className="settings-list-empty">No remote synced artifacts.</div> : null}
            </div>
          )}
        </div>
      ) : null}

      {/* ── Storage tab ───────────────────────────────────────── */}
      {tab === "storage" ? (
        <div className="settings-two-column">
          <div className="settings-subpanel">
            <h3>Projects</h3>
            <div className="settings-list">
              {projects.map((p) => (
                <button
                  className={`settings-list-item ${projectId === p.id ? "active" : ""}`}
                  key={p.id}
                  onClick={() => {
                    setProjectId(p.id);
                    setStoragePreference(p.artifactStoragePreference ?? "supabase");
                    setMessage(null);
                  }}
                  type="button"
                >
                  <strong>{p.name}</strong>
                  <span>{p.artifactStoragePreference ?? "supabase"}</span>
                </button>
              ))}
              {projects.length === 0 ? <div className="settings-list-empty">No projects.</div> : null}
            </div>
          </div>

          <div className="settings-subpanel">
            {selectedProject ? (
              <>
                <h3>{selectedProject.name} — Storage</h3>
                <div className="settings-grid">
                  <label className="settings-field">
                    <span>Storage Provider</span>
                    <select
                      value={storagePreference}
                      onChange={(e) => setStoragePreference(e.target.value as "supabase" | "google_drive")}
                    >
                      <option value="supabase">Supabase (default)</option>
                      <option value="google_drive">Google Drive</option>
                    </select>
                  </label>
                </div>

                {storagePreference === "supabase" ? (
                  <div className="artifact-info-panel">
                    <strong>Supabase Storage is active</strong>
                    <p>Artifacts for this project sync to the shared Supabase bucket automatically. No additional driver configuration required.</p>
                  </div>
                ) : driver ? (
                  <>
                    <h4 style={{ marginTop: "16px" }}>Google Drive Driver</h4>
                    <div className="settings-grid">
                      <label className="settings-field">
                        <span>Remote Root</span>
                        <input
                          value={driver.remoteRootPath}
                          onChange={(e) => setDriver((d) => d ? { ...d, remoteRootPath: e.target.value } : d)}
                        />
                      </label>
                      <label className="settings-field">
                        <span>Folder Name</span>
                        <input
                          value={driver.remoteFolderName}
                          onChange={(e) => setDriver((d) => d ? { ...d, remoteFolderName: e.target.value } : d)}
                        />
                      </label>
                      <label className="settings-checkbox">
                        <input
                          checked={driver.enabled}
                          type="checkbox"
                          onChange={(e) => setDriver((d) => d ? { ...d, enabled: e.target.checked } : d)}
                        />
                        <span>Enabled</span>
                      </label>
                    </div>
                  </>
                ) : null}

                <div className="settings-actions" style={{ marginTop: "16px" }}>
                  <button className="primary-btn" disabled={busy} onClick={() => void saveStorage()} type="button">
                    Save Storage
                  </button>
                </div>
              </>
            ) : (
              <div className="settings-list-empty">Select a project to configure storage.</div>
            )}
          </div>
        </div>
      ) : null}

    </section>
  );
}
