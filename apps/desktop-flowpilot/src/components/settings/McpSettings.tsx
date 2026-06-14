import { useEffect, useState } from "react";
import type { Integration, IntegrationType, LocalRunnerMcpBackend, Project } from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { buildConfig, createEmptyConfig, formatTimestamp, integrationTypes, providerFields, toErrorMessage } from "@/components/settings/settingsHelpers";

type McpSettingsMode = "all" | "google-drive" | "jira";

interface McpSettingsProps {
  mode?: McpSettingsMode;
}

function modeLabel(mode: McpSettingsMode) {
  if (mode === "google-drive") return "Google Drive";
  if (mode === "jira") return "Jira MCP";
  return "MCP";
}

function allowedTypes(mode: McpSettingsMode): IntegrationType[] {
  if (mode === "google-drive") return ["google_drive"];
  if (mode === "jira") return ["jira"];
  return integrationTypes;
}

export function McpSettings({ mode = "all" }: McpSettingsProps): React.ReactElement {
  const types = allowedTypes(mode);
  const [projects, setProjects] = useState<Project[]>([]);
  const [integrations, setIntegrations] = useState<Integration[]>([]);
  const [backends, setBackends] = useState<LocalRunnerMcpBackend[]>([]);
  const [projectId, setProjectId] = useState("");
  const [type, setType] = useState<IntegrationType>(types[0] ?? "jira");
  const [label, setLabel] = useState(types[0] === "google_drive" ? "Google Drive MCP" : "Jira MCP");
  const [config, setConfig] = useState<Record<string, string>>(createEmptyConfig(types[0] ?? "jira"));
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);

  const refresh = async () => {
    try {
      const admin = await getAdminUseCases();
      const [nextProjects, nextIntegrations, nextBackends] = await Promise.all([
        admin.projects.listProjects(),
        admin.integrations.listIntegrations(),
        admin.integrations.listMcpBackends(),
      ]);
      setProjects(nextProjects);
      setIntegrations(nextIntegrations.filter((integration) => types.includes(integration.type)));
      setBackends(nextBackends);
      setProjectId((current) => current || nextProjects[0]?.id || "");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to load MCP settings."));
    }
  };

  useEffect(() => {
    void refresh();
  }, []);

  const changeType = (nextType: IntegrationType) => {
    setType(nextType);
    setConfig(createEmptyConfig(nextType));
    setLabel(nextType === "jira" ? "Jira MCP" : nextType === "google_drive" ? "Google Drive MCP" : `${nextType} MCP`);
  };

  useEffect(() => {
    const firstType = types[0] ?? "jira";
    setType(firstType);
    setConfig(createEmptyConfig(firstType));
    setLabel(firstType === "google_drive" ? "Google Drive MCP" : "Jira MCP");
  }, [mode]);

  const createIntegration = async () => {
    if (!projectId) return;
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      await admin.integrations.createIntegration({
        projectId,
        type,
        label,
        configEncrypted: buildConfig(type, config),
        status: type === "jira" ? "pending" : "connected",
        mcpTypeEnabled: true,
      });
      await refresh();
      setMessage("MCP integration created.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to create MCP integration."));
    } finally {
      setBusy(false);
    }
  };

  const runBackendAction = async (backend: LocalRunnerMcpBackend) => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      await admin.integrations.runMcpBackendAction(backend.key, backend.action, projectId);
      await refresh();
      setMessage(`${backend.label} action completed.`);
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to run MCP backend action."));
    } finally {
      setBusy(false);
    }
  };

  const testIntegration = async (integration: Integration) => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      const result = await admin.integrations.testIntegration(
        integration.projectId,
        integration.id,
        integration.type,
        integration.configEncrypted as Record<string, string>,
      );
      await refresh();
      setMessage(result ?? "Integration test started.");
    } catch (error) {
      setMessage(toErrorMessage(error, "Unable to test MCP integration."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="settings-panel">
      <div className="settings-panel-head"><div><div className="settings-eyebrow">MCP</div><h2>{modeLabel(mode)}</h2><p>Manage runner MCP backends and project-bound MCP integrations.</p></div></div>
      {message ? <div className="settings-feedback">{message}</div> : null}
      <div className="settings-two-column">
        <div className="settings-subpanel"><h3>Runner Backends</h3><div className="settings-list">{backends.map((backend) => <div className="settings-list-item static" key={backend.key}><div><strong>{backend.label}</strong><span>{backend.providerType} / {backend.state} / {formatTimestamp(backend.lastCheckedAt)}</span></div><button className="secondary-btn" disabled={busy || !projectId} onClick={() => void runBackendAction(backend)} type="button">{backend.actionLabel}</button></div>)}</div></div>
        <div className="settings-subpanel"><h3>Create Integration</h3><div className="settings-grid"><label className="settings-field"><span>Project</span><select value={projectId} onChange={(event) => setProjectId(event.target.value)}>{projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select></label>{mode === "all" ? <label className="settings-field"><span>Type</span><select value={type} onChange={(event) => changeType(event.target.value as IntegrationType)}>{types.map((option) => <option key={option} value={option}>{option}</option>)}</select></label> : null}<label className="settings-field settings-field-full"><span>Label</span><input value={label} onChange={(event) => setLabel(event.target.value)} /></label>{providerFields[type].map((field) => <label className="settings-field" key={field.key}><span>{field.label}</span><input type={field.type ?? "text"} value={config[field.key] ?? ""} onChange={(event) => setConfig((current) => ({ ...current, [field.key]: event.target.value }))} /></label>)}</div><div className="settings-actions"><button className="primary-btn" disabled={busy || !projectId} onClick={() => void createIntegration()} type="button">Create Integration</button></div></div>
      </div>
      <div className="settings-subpanel"><h3>Existing Integrations</h3><div className="settings-list">{integrations.map((integration) => <div className="settings-list-item static" key={integration.id}><div><strong>{integration.label}</strong><span>{integration.type} / {integration.status} / project {integration.projectId}</span></div>{integration.type === "jira" || integration.type === "google_drive" ? <button className="secondary-btn" disabled={busy} onClick={() => void testIntegration(integration)} type="button">Test</button> : null}</div>)}</div></div>
    </section>
  );
}
