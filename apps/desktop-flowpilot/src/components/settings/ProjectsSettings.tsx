import { useEffect, useMemo, useState } from "react";
import {
  assertValidProjectBindings,
  resolveProviderKeyForModel,
  type Integration,
  type Project,
  type ProjectPlatform,
  type ProjectWorkspaceBinding,
  type SupportedModel,
  type Team,
  type WorkflowRun,
} from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { formatTimestamp, integrationTypes, toErrorMessage, validateDirectoryBindingsInOrder } from "@/components/settings/settingsHelpers";

type BindingDraft = Pick<ProjectWorkspaceBinding, "id" | "localPath" | "label"> & { persisted: boolean };

export function ProjectsSettings(): React.ReactElement {
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [teams, setTeams] = useState<Team[]>([]);
  const [models, setModels] = useState<SupportedModel[]>([]);
  const [integrations, setIntegrations] = useState<Integration[]>([]);
  const [runs, setRuns] = useState<WorkflowRun[]>([]);
  const [selectedProjectId, setSelectedProjectId] = useState("");
  const [selectedTeamIds, setSelectedTeamIds] = useState<string[]>([]);
  const [linkedIntegrationIds, setLinkedIntegrationIds] = useState<Record<string, string>>({});
  const [bindings, setBindings] = useState<BindingDraft[]>([]);
  const [createForm, setCreateForm] = useState({
    name: "",
    description: "",
    platform: "android" as ProjectPlatform,
    repositoryUrl: "",
    directoryPath: "",
  });
  const [defaults, setDefaults] = useState({
    defaultModel: "gpt-5.4",
    defaultReasoningEffort: "medium",
    sessionIdleTtlMinutes: 120,
  });

  const selectedProject = useMemo(
    () => projects.find((project) => project.id === selectedProjectId) ?? null,
    [projects, selectedProjectId],
  );

  const refresh = async (nextProjectId?: string) => {
    setLoading(true);
    try {
      const admin = await getAdminUseCases();
      const [nextProjects, nextTeams, nextModels, nextIntegrations, nextRuns] = await Promise.all([
        admin.projects.listProjects(),
        admin.teams.listTeams(),
        admin.providers.listSupportedModels(),
        admin.integrations.listIntegrations(),
        admin.workflows.listWorkflowRuns(),
      ]);
      setProjects(nextProjects);
      setTeams(nextTeams);
      setModels(nextModels);
      setIntegrations(nextIntegrations);
      setRuns(nextRuns);
      setSelectedProjectId(
        nextProjectId && nextProjects.some((project) => project.id === nextProjectId)
          ? nextProjectId
          : nextProjects[0]?.id ?? "",
      );
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to load project settings."));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (!selectedProject) return;
    void (async () => {
      try {
        const admin = await getAdminUseCases();
        const [projectTeams, projectBindings, linked] = await Promise.all([
          admin.teams.listTeamsByProject(selectedProject.id),
          admin.projects.listBindings(selectedProject.id),
          admin.integrations.listLinkedIntegrations(selectedProject.id),
        ]);
        setSelectedTeamIds(projectTeams.map((team) => team.id));
        setBindings(projectBindings.map((binding) => ({ ...binding, persisted: true })));
        setLinkedIntegrationIds(Object.fromEntries(linked.map((integration) => [integration.type, integration.id])));
        setDefaults({
          defaultModel: selectedProject.defaultModel ?? models[0]?.modelId ?? "gpt-5.4",
          defaultReasoningEffort: selectedProject.defaultReasoningEffort ?? "medium",
          sessionIdleTtlMinutes: selectedProject.sessionIdleTtlMinutes ?? 120,
        });
      } catch (error) {
        setMessage(toErrorMessage(error, "Unable to load selected project."));
      }
    })();
  }, [models, selectedProject]);

  const createProject = async () => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      assertValidProjectBindings([{ localPath: createForm.directoryPath }]);
      const project = await admin.projects.createProject({
        ...createForm,
        directoryPath: createForm.directoryPath.trim(),
        status: "active",
        artifactStoragePreference: "supabase",
      });
      await admin.projects.saveBinding(project.id, {
        localPath: createForm.directoryPath.trim(),
        label: "Primary",
      });
      setCreateForm({ name: "", description: "", platform: "android", repositoryUrl: "", directoryPath: "" });
      await refresh(project.id);
      setMessage("Project created.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to create project."));
    } finally {
      setBusy(false);
    }
  };

  const saveProjectSettings = async () => {
    if (!selectedProject) return;
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      const provider = resolveProviderKeyForModel(defaults.defaultModel, models);
      if (!provider) throw new Error("Default model is not mapped to a provider.");
      await admin.projects.updateProject(selectedProject.id, {
        defaultProvider: provider,
        defaultModel: defaults.defaultModel,
        defaultReasoningEffort: defaults.defaultReasoningEffort as Project["defaultReasoningEffort"],
        sessionIdleTtlMinutes: defaults.sessionIdleTtlMinutes,
      });
      await admin.teams.setProjectTeams(selectedProject.id, selectedTeamIds);
      for (const type of integrationTypes) {
        await admin.integrations.setProjectIntegration(selectedProject.id, type, linkedIntegrationIds[type] || null);
      }
      await refresh(selectedProject.id);
      setMessage("Project settings saved.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to save project settings."));
    } finally {
      setBusy(false);
    }
  };

  const saveBinding = async (binding: BindingDraft) => {
    if (!selectedProject) return;
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      assertValidProjectBindings(
        bindings.map((item) => ({
          localPath: item.id === binding.id ? binding.localPath : item.localPath,
        })),
      );
      await admin.projects.saveBinding(selectedProject.id, binding);
      await refresh(selectedProject.id);
      setMessage("Directory binding saved.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to save directory binding."));
    } finally {
      setBusy(false);
    }
  };

  const validateBindings = async () => {
    try {
      const admin = await getAdminUseCases();
      const usablePath = await validateDirectoryBindingsInOrder(
        bindings.map((binding) => ({
          ...binding,
          projectId: selectedProjectId,
          createdAt: "",
          updatedAt: "",
        })),
        admin.directories,
      );
      setMessage(`Usable binding: ${usablePath}`);
    } catch (error) {
      setMessage(toErrorMessage(error, "No usable binding found."));
    }
  };

  return (
    <section className="settings-panel">
      <div className="settings-panel-head">
        <div>
          <div className="settings-eyebrow">Projects</div>
          <h2>Projects</h2>
          <p>Manage projects, directory bindings, team links, defaults, and integrations.</p>
        </div>
      </div>
      {message ? <div className="settings-feedback">{message}</div> : null}
      {loading ? <div className="settings-feedback">Loading projects...</div> : null}

      <div className="settings-two-column">
        <div className="settings-subpanel">
          <h3>Create Project</h3>
          <div className="settings-grid">
            <label className="settings-field"><span>Name</span><input value={createForm.name} onChange={(event) => setCreateForm((current) => ({ ...current, name: event.target.value }))} /></label>
            <label className="settings-field"><span>Repository URL</span><input value={createForm.repositoryUrl} onChange={(event) => setCreateForm((current) => ({ ...current, repositoryUrl: event.target.value }))} /></label>
            <label className="settings-field settings-field-full"><span>Description</span><textarea value={createForm.description} onChange={(event) => setCreateForm((current) => ({ ...current, description: event.target.value }))} /></label>
            <label className="settings-field"><span>Platform</span><select value={createForm.platform} onChange={(event) => setCreateForm((current) => ({ ...current, platform: event.target.value as ProjectPlatform }))}><option value="android">android</option><option value="ios">ios</option><option value="web">web</option><option value="multi">multi</option></select></label>
            <label className="settings-field"><span>Primary Directory</span><input value={createForm.directoryPath} onChange={(event) => setCreateForm((current) => ({ ...current, directoryPath: event.target.value }))} /></label>
          </div>
          <div className="settings-actions"><button className="primary-btn" disabled={busy} onClick={() => void createProject()} type="button">Create</button></div>
        </div>

        <div className="settings-subpanel">
          <h3>Registry</h3>
          <div className="settings-list">
            {projects.map((project) => (
              <button className={`settings-list-item ${project.id === selectedProjectId ? "active" : ""}`} key={project.id} onClick={() => setSelectedProjectId(project.id)} type="button">
                <strong>{project.name}</strong>
                <span>{project.platform} / {project.status}</span>
              </button>
            ))}
          </div>
        </div>
      </div>

      {selectedProject ? (
        <>
          <div className="settings-subpanel">
            <h3>{selectedProject.name}</h3>
            <div className="settings-grid">
              <label className="settings-field"><span>Default Model</span><select value={defaults.defaultModel} onChange={(event) => setDefaults((current) => ({ ...current, defaultModel: event.target.value }))}>{models.map((model) => <option key={model.id} value={model.modelId}>{model.displayName}</option>)}</select></label>
              <label className="settings-field"><span>Reasoning</span><select value={defaults.defaultReasoningEffort} onChange={(event) => setDefaults((current) => ({ ...current, defaultReasoningEffort: event.target.value }))}><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="xhigh">Extra High</option></select></label>
              <label className="settings-field"><span>Provider Session Idle TTL</span><input min={1} type="number" value={defaults.sessionIdleTtlMinutes} onChange={(event) => setDefaults((current) => ({ ...current, sessionIdleTtlMinutes: Number(event.target.value) || 1 }))} /></label>
            </div>
          </div>

          <div className="settings-two-column">
            <div className="settings-subpanel">
              <h3>Directory Bindings</h3>
              <div className="settings-list">
                {bindings.map((binding, index) => (
                  <div className="settings-card" key={binding.id}>
                    <div className="settings-card-head"><strong>{index === 0 ? "Primary" : `Binding ${index + 1}`}</strong><span>{binding.persisted ? "saved" : "new"}</span></div>
                    <div className="settings-grid">
                      <label className="settings-field"><span>Path</span><input value={binding.localPath} onChange={(event) => setBindings((current) => current.map((item) => item.id === binding.id ? { ...item, localPath: event.target.value } : item))} /></label>
                      <label className="settings-field"><span>Label</span><input value={binding.label ?? ""} onChange={(event) => setBindings((current) => current.map((item) => item.id === binding.id ? { ...item, label: event.target.value } : item))} /></label>
                    </div>
                    <div className="settings-actions"><button className="secondary-btn" disabled={busy} onClick={() => void saveBinding(binding)} type="button">Save</button></div>
                  </div>
                ))}
              </div>
              <div className="settings-actions">
                <button className="secondary-btn" onClick={() => setBindings((current) => [...current, { id: crypto.randomUUID(), localPath: "", label: "", persisted: false }])} type="button">Add Binding</button>
                <button className="secondary-btn" onClick={() => void validateBindings()} type="button">Validate Fallback</button>
              </div>
            </div>

            <div className="settings-subpanel">
              <h3>Teams</h3>
              <div className="settings-checkbox-list">
                {teams.map((team) => (
                  <label className="settings-checkbox" key={team.id}><input checked={selectedTeamIds.includes(team.id)} onChange={() => setSelectedTeamIds((current) => current.includes(team.id) ? current.filter((id) => id !== team.id) : [...current, team.id])} type="checkbox" /><span>{team.name}</span></label>
                ))}
              </div>
            </div>
          </div>

          <div className="settings-two-column">
            <div className="settings-subpanel">
              <h3>Integrations</h3>
              <div className="settings-grid">
                {integrationTypes.map((type) => (
                  <label className="settings-field" key={type}><span>{type}</span><select value={linkedIntegrationIds[type] ?? ""} onChange={(event) => setLinkedIntegrationIds((current) => ({ ...current, [type]: event.target.value }))}><option value="">Not linked</option>{integrations.filter((integration) => integration.type === type).map((integration) => <option key={integration.id} value={integration.id}>{integration.label}</option>)}</select></label>
                ))}
              </div>
            </div>
            <div className="settings-subpanel">
              <h3>Run History</h3>
              <div className="settings-list">{runs.filter((run) => run.projectId === selectedProject.id).slice(0, 8).map((run) => <div className="settings-list-item static" key={run.id}><strong>{run.status}</strong><span>{run.model ?? "model"} / {formatTimestamp(run.startedAt)}</span></div>)}</div>
            </div>
          </div>

          <div className="settings-actions"><button className="primary-btn" disabled={busy} onClick={() => void saveProjectSettings()} type="button">Save Project Settings</button></div>
        </>
      ) : null}
    </section>
  );
}
