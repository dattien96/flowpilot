import { useEffect, useMemo, useState } from "react";
import type { ArtifactDefinition, ArtifactRun, LocalRunnerArtifact, LocalRunnerStorageDriver, Project } from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { formatTimestamp, toErrorMessage } from "@/components/settings/settingsHelpers";

type Tab = "generated" | "storage" | "catalog";

export function ArtifactsSettings(): React.ReactElement {
  const [tab, setTab] = useState<Tab>("generated");
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectId] = useState("");
  const [localArtifacts, setLocalArtifacts] = useState<LocalRunnerArtifact[]>([]);
  const [runs, setRuns] = useState<ArtifactRun[]>([]);
  const [definitions, setDefinitions] = useState<ArtifactDefinition[]>([]);
  const [driver, setDriver] = useState<LocalRunnerStorageDriver | null>(null);
  const [draft, setDraft] = useState<ArtifactDefinition | null>(null);
  const [storagePreference, setStoragePreference] = useState<"supabase" | "google_drive">("supabase");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);

  const refresh = async () => {
    try {
      const admin = await getAdminUseCases();
      const [nextProjects, nextLocal, nextRuns, nextDefinitions, nextDriver] = await Promise.all([
        admin.projects.listProjects(),
        admin.artifacts.listLocalArtifacts(),
        admin.artifacts.listRuns(),
        admin.artifacts.listDefinitions(),
        admin.artifacts.getStorageDriver(),
      ]);
      setProjects(nextProjects);
      setLocalArtifacts(nextLocal);
      setRuns(nextRuns);
      setDefinitions(nextDefinitions);
      setDriver(nextDriver);
      const nextProjectId = projectId || nextProjects[0]?.id || "";
      setProjectId(nextProjectId);
      const project = nextProjects.find((item) => item.id === nextProjectId) ?? nextProjects[0] ?? null;
      setStoragePreference(project?.artifactStoragePreference ?? "supabase");
      setDraft((current) => current ?? nextDefinitions[0] ?? null);
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to load artifacts."));
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

  const visibleLocalArtifacts = useMemo(() => localArtifacts.filter((item) => !projectId || item.projectId === projectId), [localArtifacts, projectId]);
  const visibleRuns = useMemo(() => runs.filter((item) => !projectId || item.projectId === projectId), [runs, projectId]);

  const createDefinition = () => {
    setDraft({
      key: `artifact_${Date.now()}`,
      name: "New Artifact",
      description: "",
      localPathTemplate: "",
      remotePathTemplate: "",
      defaultFileName: "artifact.md",
      createdAt: "",
      updatedAt: "",
    });
  };

  const saveDefinition = async () => {
    if (!draft) return;
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      const saved = await admin.artifacts.saveDefinition(draft);
      await refresh();
      setDraft(saved);
      setMessage("Artifact definition saved.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to save artifact definition."));
    } finally {
      setBusy(false);
    }
  };

  const saveStorage = async () => {
    if (!driver) return;
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      if (driver.enabled && (!driver.remoteRootPath.trim() || !driver.remoteFolderName.trim())) {
        throw new Error("Remote root path and folder name are required before enabling sync.");
      }
      if (!projectId) {
        throw new Error("Select a project before saving storage settings.");
      }
      await admin.projects.updateProject(projectId, {
        artifactStoragePreference: storagePreference,
      });
      await admin.artifacts.saveStorageDriver(driver);
      await refresh();
      setMessage("Storage driver saved.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to save storage driver."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="settings-panel">
      <div className="settings-panel-head"><div><div className="settings-eyebrow">Artifacts</div><h2>Artifacts</h2><p>Browse generated artifacts, configure storage sync, and manage artifact definitions.</p></div><div className="header-tabs"><button className={`header-tab ${tab === "generated" ? "active" : ""}`} onClick={() => setTab("generated")} type="button">Generated</button><button className={`header-tab ${tab === "storage" ? "active" : ""}`} onClick={() => setTab("storage")} type="button">Storage</button><button className={`header-tab ${tab === "catalog" ? "active" : ""}`} onClick={() => setTab("catalog")} type="button">Catalog</button></div></div>
      {message ? <div className="settings-feedback">{message}</div> : null}
      {tab === "generated" ? <div className="settings-subpanel"><div className="settings-actions"><select value={projectId} onChange={(event) => setProjectId(event.target.value)}><option value="">All projects</option>{projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select></div><div className="settings-list">{visibleLocalArtifacts.map((item) => <div className="settings-list-item static" key={item.artifactId}><strong>{item.title}</strong><span>{item.syncStatus} / {formatTimestamp(item.updatedAt)}</span></div>)}{visibleRuns.map((item) => <div className="settings-list-item static" key={item.id}><strong>{item.title}</strong><span>{item.storageProvider ?? "local"} / {item.syncStatus}</span></div>)}</div></div> : null}
      {tab === "storage" && driver ? <div className="settings-subpanel"><h3>Storage Driver</h3><div className="settings-grid"><label className="settings-field"><span>Project</span><select value={projectId} onChange={(event) => { const nextProjectId = event.target.value; setProjectId(nextProjectId); const project = projects.find((item) => item.id === nextProjectId); setStoragePreference(project?.artifactStoragePreference ?? "supabase"); }}>{projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select></label><label className="settings-field"><span>Storage Preference</span><select value={storagePreference} onChange={(event) => setStoragePreference(event.target.value as "supabase" | "google_drive")}><option value="supabase">supabase</option><option value="google_drive">google_drive</option></select></label><label className="settings-field"><span>Driver</span><input value={driver.driverKey} onChange={(event) => setDriver((current) => current ? { ...current, driverKey: event.target.value } : current)} /></label><label className="settings-field"><span>Remote Root</span><input value={driver.remoteRootPath} onChange={(event) => setDriver((current) => current ? { ...current, remoteRootPath: event.target.value } : current)} /></label><label className="settings-field"><span>Folder</span><input value={driver.remoteFolderName} onChange={(event) => setDriver((current) => current ? { ...current, remoteFolderName: event.target.value } : current)} /></label><label className="settings-checkbox"><input checked={driver.enabled} onChange={(event) => setDriver((current) => current ? { ...current, enabled: event.target.checked } : current)} type="checkbox" /><span>Enabled</span></label></div><div className="settings-actions"><button className="primary-btn" disabled={busy} onClick={() => void saveStorage()} type="button">Save Storage</button></div></div> : null}
      {tab === "catalog" ? <div className="settings-two-column"><div className="settings-subpanel"><div className="settings-actions"><button className="primary-btn" onClick={createDefinition} type="button">Create Definition</button></div><div className="settings-list">{definitions.map((definition) => <button className={`settings-list-item ${draft?.key === definition.key ? "active" : ""}`} key={definition.key} onClick={() => setDraft(definition)} type="button"><strong>{definition.name}</strong><span>{definition.key}</span></button>)}</div></div><div className="settings-subpanel"><h3>Edit Definition</h3>{draft ? <div className="settings-grid"><label className="settings-field"><span>Key</span><input value={draft.key} onChange={(event) => setDraft((current) => current ? { ...current, key: event.target.value } : current)} /></label><label className="settings-field"><span>Name</span><input value={draft.name} onChange={(event) => setDraft((current) => current ? { ...current, name: event.target.value } : current)} /></label><label className="settings-field settings-field-full"><span>Description</span><textarea value={draft.description} onChange={(event) => setDraft((current) => current ? { ...current, description: event.target.value } : current)} /></label><label className="settings-field"><span>Local Path Template</span><input value={draft.localPathTemplate} onChange={(event) => setDraft((current) => current ? { ...current, localPathTemplate: event.target.value } : current)} /></label><label className="settings-field"><span>Remote Path Template</span><input value={draft.remotePathTemplate} onChange={(event) => setDraft((current) => current ? { ...current, remotePathTemplate: event.target.value } : current)} /></label><label className="settings-field"><span>Default File Name</span><input value={draft.defaultFileName} onChange={(event) => setDraft((current) => current ? { ...current, defaultFileName: event.target.value } : current)} /></label></div> : null}<div className="settings-actions"><button className="primary-btn" disabled={busy || !draft} onClick={() => void saveDefinition()} type="button">Save Definition</button></div></div></div> : null}
    </section>
  );
}
