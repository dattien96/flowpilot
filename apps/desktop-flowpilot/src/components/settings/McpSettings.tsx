import { useEffect, useState } from "react";
import type { Integration, IntegrationType, LocalRunnerMcpBackend, Project, TelegramApprovalRecord } from "@flowpilot/client-core";
import { getAdminUseCases } from "@/clientCore";
import { RUNNER_URL } from "@/config";
import { buildConfig, createEmptyConfig, findDuplicateJiraIntegration, formatTimestamp, integrationTypes, providerFields, stripSecretFields, toErrorMessage } from "@/components/settings/settingsHelpers";

// Task-228/230/232 shipped EnsureClaude{Jira,Firebase,Telegram}McpConfig but
// no HTTP route or UI ever called them — Google Drive is the only one of the
// four MCP integrations whose provider-config write-up actually ran in
// production, via a "Configure Providers" loop button (GoogleDriveSettings.tsx
// configureAiProviders) that walks every discovered account of all four
// provider CLIs. Task-234 T-2/T-4 gives Jira/Firebase/Telegram the same
// button, reusing the same /provider-accounts listing endpoint and looping
// over every account regardless of provider_key (not a single manual pick).
interface RunnerProviderAccount {
  provider_key: string;
  home_path: string;
  display_name: string;
  is_active: boolean;
}

const PROVIDER_LABELS: Record<string, string> = { codex: "Codex", gemini: "Gemini", claude: "Claude", grok: "Grok" };

interface McpProviderConfigResult {
  providerKey: string;
  serverName: string;
  status: string;
  changed: boolean;
  configPath: string;
}

function runnerFetch(path: string, init?: RequestInit): Promise<Response> {
  return fetch(new URL(path, RUNNER_URL).toString(), { cache: "no-store", ...init });
}

async function readRunnerError(response: Response): Promise<string> {
  const text = await response.text().catch(() => "");
  if (!text) return `Request failed with status ${response.status}.`;
  try {
    const payload = JSON.parse(text) as { error?: string };
    return payload.error || text;
  } catch {
    return text;
  }
}

const providerConfigEnsurePath: Partial<Record<IntegrationType, string>> = {
  jira: "/jira-config/mcp-provider-config/ensure",
  firebase: "/firebase-config/mcp-provider-config/ensure",
  telegram: "/telegram-config/mcp-provider-config/ensure",
};

type McpSettingsMode = "all" | "google-drive" | "jira" | "firebase" | "telegram";
type IntegrationOwnerScope = "global" | "project";

interface McpSettingsProps {
  mode?: McpSettingsMode;
}

function modeLabel(mode: McpSettingsMode) {
  if (mode === "google-drive") return "Google Drive";
  if (mode === "jira") return "Jira MCP";
  if (mode === "firebase") return "Firebase MCP";
  if (mode === "telegram") return "Telegram MCP";
  return "MCP";
}

function allowedTypes(mode: McpSettingsMode): IntegrationType[] {
  if (mode === "google-drive") return ["google_drive"];
  if (mode === "jira") return ["jira"];
  if (mode === "firebase") return ["firebase"];
  if (mode === "telegram") return ["telegram"];
  return integrationTypes;
}

function labelForIntegrationType(type: IntegrationType): string {
  if (type === "jira") return "Jira MCP";
  if (type === "google_drive") return "Google Drive MCP";
  if (type === "firebase") return "Firebase MCP";
  if (type === "telegram") return "Telegram MCP";
  return `${type} MCP`;
}

export function McpSettings({ mode = "all" }: McpSettingsProps): React.ReactElement {
  const types = allowedTypes(mode);
  const [projects, setProjects] = useState<Project[]>([]);
  const [integrations, setIntegrations] = useState<Integration[]>([]);
  const [backends, setBackends] = useState<LocalRunnerMcpBackend[]>([]);
  const [projectId, setProjectId] = useState("");
  const [ownerScope, setOwnerScope] = useState<IntegrationOwnerScope>("global");
  const [type, setType] = useState<IntegrationType>(types[0] ?? "jira");
  const [label, setLabel] = useState(labelForIntegrationType(types[0] ?? "jira"));
  const [config, setConfig] = useState<Record<string, string>>(createEmptyConfig(types[0] ?? "jira"));
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  // Errors surface in a blocking modal (settings-modal pattern) so a failed
  // Jira verify / token rejection can't be missed as inline feedback text.
  const [errorModal, setErrorModal] = useState<{ title: string; body: string } | null>(null);
  const [editingIntegrationId, setEditingIntegrationId] = useState<string | null>(null);
  const [telegramApprovals, setTelegramApprovals] = useState<TelegramApprovalRecord[]>([]);
  // Task-233 / CLI E2E: when true, telegram-mcp skips the pending-approval
  // queue for out-of-run-scope calls (Codex CLI standalone). Stored in the
  // runner keyring credential as autoApprove (not Supabase secrets).
  const [telegramAutoApprove, setTelegramAutoApprove] = useState(false);
  const [providerAccounts, setProviderAccounts] = useState<RunnerProviderAccount[]>([]);
  const [configureMessage, setConfigureMessage] = useState<string | null>(null);

  const ensurePath = providerConfigEnsurePath[type];
  const supportsGlobalScope = type === "jira" || type === "firebase" || type === "telegram";
  const ownerProjectId = supportsGlobalScope && ownerScope === "global" ? null : (projectId || null);
  const editingIntegration = editingIntegrationId
    ? integrations.find((integration) => integration.id === editingIntegrationId) ?? null
    : null;

  const refresh = async (allowed: IntegrationType[] = types) => {
    try {
      const admin = await getAdminUseCases();
      const [nextProjects, nextIntegrations, nextBackends, nextTelegramApprovals] = await Promise.all([
        admin.projects.listProjects(),
        admin.integrations.listIntegrations(),
        admin.integrations.listMcpBackends(),
        // Only Telegram cares about the pending-send queue; skip the call on
        // other single-type modes so a slow/failed approvals fetch cannot
        // block Jira/Firebase pages (and never surface Telegram-only UI there).
        mode === "telegram" || mode === "all"
          ? admin.integrations.listTelegramProxyApprovals("pending")
          : Promise.resolve([] as TelegramApprovalRecord[]),
      ]);
      setProjects(nextProjects);
      setIntegrations(nextIntegrations.filter((integration) => allowed.includes(integration.type)));
      setBackends(
        nextBackends.filter(
          (backend) =>
            allowed.includes(backend.providerType as IntegrationType) ||
            allowed.includes(backend.key as IntegrationType),
        ),
      );
      setTelegramApprovals(nextTelegramApprovals);
      setProjectId((current) => current || nextProjects[0]?.id || "");
      if (providerConfigEnsurePath[allowed[0] ?? "jira"]) {
        const accountsResponse = await runnerFetch("/provider-accounts");
        if (accountsResponse.ok) {
          const payload = (await accountsResponse.json()) as { accounts?: RunnerProviderAccount[] };
          setProviderAccounts((payload.accounts ?? []).filter((account) => account.home_path?.trim().length > 0));
        }
      } else {
        setProviderAccounts([]);
      }
    } catch (error) {
      setErrorModal({ title: "Unable to load MCP settings", body: toErrorMessage(error, "Unable to load MCP settings.") });
    }
  };

  const changeType = (nextType: IntegrationType) => {
    setEditingIntegrationId(null);
    setType(nextType);
    setConfig(createEmptyConfig(nextType));
    setLabel(labelForIntegrationType(nextType));
    setOwnerScope(nextType === "jira" || nextType === "firebase" || nextType === "telegram" ? "global" : "project");
    setTelegramAutoApprove(false);
  };

  // Reset editor + re-fetch scoped lists whenever the Settings nav page changes.
  // SettingsShell reuses <McpSettings> across jira/firebase/telegram (same
  // component type); without this, integrations/backends/form fields from the
  // previous tab stay mounted and look "swapped" (e.g. Telegram form on Jira).
  useEffect(() => {
    const firstType = types[0] ?? "jira";
    setType(firstType);
    setConfig(createEmptyConfig(firstType));
    setLabel(labelForIntegrationType(firstType));
    setOwnerScope(firstType === "jira" || firstType === "firebase" || firstType === "telegram" ? "global" : "project");
    setEditingIntegrationId(null);
    setTelegramAutoApprove(false);
    setMessage(null);
    setConfigureMessage(null);
    setErrorModal(null);
    // Drop stale lists immediately so the previous tab's rows don't flash.
    setIntegrations([]);
    setBackends([]);
    setTelegramApprovals([]);
    setProviderAccounts([]);
    void refresh(types);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- intentionally mode-scoped; refresh closes over latest types via arg
  }, [mode]);

  const beginEditIntegration = (integration: Integration) => {
    setEditingIntegrationId(integration.id);
    setType(integration.type);
    setLabel(integration.label);
    setOwnerScope(integration.projectId ? "project" : "global");
    setProjectId(integration.projectId ?? projects[0]?.id ?? "");
    const nextConfig = createEmptyConfig(integration.type);
    for (const field of providerFields[integration.type]) {
      if (integration.type === "jira" && field.key === "apiToken") continue;
      const raw = (integration.configEncrypted as Record<string, unknown>)[field.key];
      nextConfig[field.key] = typeof raw === "string" ? raw : "";
    }
    setConfig(nextConfig);
    // UI mirror of keyring autoApprove (non-secret); source of truth is runner keyring.
    const mirror = (integration.configEncrypted as Record<string, unknown>).autoApprove;
    setTelegramAutoApprove(mirror === true || mirror === "true");
    setMessage(null);
  };

  const resetEditor = () => {
    const nextType = types[0] ?? "jira";
    setEditingIntegrationId(null);
    setType(nextType);
    setConfig(createEmptyConfig(nextType));
    setLabel(labelForIntegrationType(nextType));
    setOwnerScope(nextType === "jira" || nextType === "firebase" || nextType === "telegram" ? "global" : "project");
    setProjectId((current) => current || projects[0]?.id || "");
    setTelegramAutoApprove(false);
  };

  const createIntegration = async () => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      const fullConfig = buildConfig(type, config);
      const needsConnect = type === "jira" || type === "firebase" || type === "telegram";
      const duplicate = type === "jira" && !editingIntegration
        ? findDuplicateJiraIntegration(integrations, ownerProjectId, String(fullConfig.workspaceUrl ?? ""))
        : null;
      // autoApprove is a non-secret UI mirror only; the runner keyring holds
      // the authoritative flag (telegramCredential.autoApprove).
      const persistedConfig =
        type === "telegram"
          ? { ...stripSecretFields(type, fullConfig), autoApprove: telegramAutoApprove }
          : stripSecretFields(type, fullConfig);
      const created = editingIntegration
        ? await admin.integrations.updateIntegration(editingIntegration.id, {
          label,
          configEncrypted: persistedConfig,
          status: needsConnect ? "pending" : editingIntegration.status,
          mcpTypeEnabled: editingIntegration.mcpTypeEnabled,
        })
        : duplicate
        ? await admin.integrations.updateIntegration(duplicate.id, {
          label,
          configEncrypted: persistedConfig,
          status: needsConnect ? "pending" : "connected",
          mcpTypeEnabled: true,
        })
        : await admin.integrations.createIntegration({
          projectId: ownerProjectId,
          type,
          label,
          // SD-11 §6 secret boundary: the raw credential (apiToken /
          // serviceAccountJson) must never land in Supabase config_encrypted —
          // only the runner keyring. It is sent to the runner once, below, via
          // testIntegration (which posts to /integrations/{id}/connection), never
          // persisted here.
          configEncrypted: persistedConfig,
          status: needsConnect ? "pending" : "connected",
          mcpTypeEnabled: true,
        });
      if (needsConnect) {
        const connectFields: Record<string, string | boolean | undefined> = {
          ...(fullConfig as Record<string, string>),
        };
        if (type === "telegram") {
          connectFields.telegramAutoApprove = telegramAutoApprove;
        }
        const outcome = await admin.integrations.testIntegration(undefined, created.id, type, connectFields);
        await admin.integrations.updateIntegration(created.id, { status: outcome.ok ? "connected" : "failed" });
        if (!outcome.ok) {
          setErrorModal({ title: "Connection failed", body: outcome.message ?? "The runner rejected the connection." });
          await refresh();
          return;
        }
        setMessage(outcome.message ?? (editingIntegration ? "MCP integration updated and reconnected." : duplicate ? "Existing MCP integration reconnected." : "MCP integration created and connected."));
      } else {
        setMessage(editingIntegration ? "MCP integration updated." : duplicate ? "Existing MCP integration updated." : "MCP integration created.");
      }
      resetEditor();
      await refresh();
    } catch (error) {
      setErrorModal({ title: editingIntegration ? "Unable to update integration" : "Unable to create integration", body: toErrorMessage(error, `Unable to ${editingIntegration ? "update" : "create"} MCP integration.`) });
    } finally {
      setBusy(false);
    }
  };

  // Toggle keyring autoApprove without re-pasting the bot token (reuses the
  // stored credential; sends only telegramAutoApprove pointer to the runner).
  const setTelegramAutoApproveOnIntegration = async (integration: Integration, enabled: boolean) => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      const outcome = await admin.integrations.testIntegration(undefined, integration.id, "telegram", {
        telegramAutoApprove: enabled,
      });
      if (!outcome.ok) {
        setErrorModal({
          title: "Unable to update auto-approve",
          body: outcome.message ?? "The runner rejected the auto-approve update.",
        });
        return;
      }
      await admin.integrations.updateIntegration(integration.id, {
        configEncrypted: {
          ...(integration.configEncrypted as Record<string, unknown>),
          autoApprove: enabled,
        },
      });
      await refresh();
      setMessage(
        enabled
          ? "Telegram auto-approve enabled (CLI / no-scope sends will go through without Pending Approvals)."
          : "Telegram auto-approve disabled (CLI sends require this flag or a FlowPilot run approval).",
      );
    } catch (error) {
      setErrorModal({
        title: "Unable to update auto-approve",
        body: toErrorMessage(error, "Unable to update Telegram auto-approve."),
      });
    } finally {
      setBusy(false);
    }
  };

  // configureProviders (Task-234 T-4) loops every discovered account across
  // all four provider CLIs and writes this MCP's config into each — mirrors
  // GoogleDriveSettings.tsx's configureAiProviders Step-7 loop button exactly,
  // instead of a single manual account pick.
  const configureProviders = async () => {
    if (!ensurePath || providerAccounts.length === 0) return;
    setBusy(true);
    setConfigureMessage(null);
    const errors: string[] = [];
    for (const account of providerAccounts) {
      try {
        // Jira MCP uses the connected email + API token (Basic auth against the
        // Rovo MCP server); no OAuth/service-account Bearer override is sent.
        const body: Record<string, string> = { providerKey: account.provider_key, accountHomePath: account.home_path };
        const response = await runnerFetch(ensurePath, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        });
        if (!response.ok) {
          throw new Error(await readRunnerError(response));
        }
      } catch (error) {
        errors.push(`${PROVIDER_LABELS[account.provider_key] ?? account.provider_key} (${account.display_name || account.home_path}): ${toErrorMessage(error, "Unknown error")}`);
      }
    }
    await refresh();
    if (errors.length > 0) {
      setErrorModal({ title: "Configure Providers failed", body: errors.join("\n") });
    } else {
      setConfigureMessage(`Configured ${providerAccounts.length} account${providerAccounts.length === 1 ? "" : "s"} successfully.`);
    }
    setBusy(false);
  };

  const runBackendAction = async (backend: LocalRunnerMcpBackend) => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      await admin.integrations.runMcpBackendAction(backend.key, backend.action);
      await refresh();
      setMessage(`${backend.label} action completed.`);
    } catch (error) {
      setErrorModal({ title: `${backend.label} action failed`, body: toErrorMessage(error, "Unable to run MCP backend action.") });
    } finally {
      setBusy(false);
    }
  };

  const testIntegration = async (integration: Integration) => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      const outcome = await admin.integrations.testIntegration(
        undefined,
        integration.id,
        integration.type,
        integration.configEncrypted as Record<string, string>,
      );
      await refresh();
      if (!outcome.ok) {
        setErrorModal({ title: `${integration.label} test failed`, body: outcome.message ?? "The runner rejected the connection." });
        return;
      }
      setMessage(outcome.message ?? "Integration test started.");
    } catch (error) {
      setErrorModal({ title: "Unable to test integration", body: toErrorMessage(error, "Unable to test MCP integration.") });
    } finally {
      setBusy(false);
    }
  };

  const deleteIntegration = async (integration: Integration) => {
    if (!window.confirm(`Delete integration "${integration.label}"?`)) return;
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      if (integration.type === "jira" || integration.type === "google_drive" || integration.type === "firebase" || integration.type === "telegram") {
        const response = await runnerFetch(`/integrations/${encodeURIComponent(integration.id)}/connection`, { method: "DELETE" });
        if (!response.ok) {
          throw new Error(await readRunnerError(response));
        }
      }
      await admin.integrations.deleteIntegration(integration.id);
      await refresh();
      setMessage("Integration deleted.");
    } catch (error) {
      setErrorModal({ title: "Unable to delete integration", body: toErrorMessage(error, "Unable to delete MCP integration.") });
    } finally {
      setBusy(false);
    }
  };

  const renderFieldGuide = (field: (typeof providerFields)[IntegrationType][number]) => {
    if (!field.helpTitle && !field.helpBody && !field.helpItems?.length && !field.helpLinks?.length) return null;
    return (
      <details className="mcp-field-guide">
        <summary>{field.helpTitle ?? `How to get ${field.label}`}</summary>
        {field.helpBody ? <p>{field.helpBody}</p> : null}
        {field.helpItems?.length ? (
          <ul className="mcp-field-guide-list">
            {field.helpItems.map((item) => (
              <li key={item}>{item}</li>
            ))}
          </ul>
        ) : null}
        {field.helpLinks?.length ? (
          <div className="mcp-field-guide-links">
            {field.helpLinks.map((link) => (
              <a href={link.href} key={link.href} rel="noreferrer" target="_blank">
                {link.label}
              </a>
            ))}
          </div>
        ) : null}
      </details>
    );
  };

  const decideTelegramApproval = async (id: string, decision: "approved" | "rejected") => {
    setBusy(true);
    try {
      const admin = await getAdminUseCases();
      await admin.integrations.decideTelegramProxyApproval(id, decision);
      await refresh();
      setMessage(decision === "approved" ? "Telegram message approved. It will send on the AI's next retry." : "Telegram message rejected.");
    } catch (error) {
      setErrorModal({ title: "Unable to decide Telegram approval", body: toErrorMessage(error, "Unable to decide Telegram approval.") });
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="settings-panel">
      <div className="settings-panel-head"><div><div className="settings-eyebrow">MCP</div><h2>{modeLabel(mode)}</h2><p>Manage runner MCP backends and reusable MCP integrations that can be linked to any project or step later.</p></div></div>
      {message ? <div className="settings-feedback">{message}</div> : null}
      <div className="settings-two-column">
        <div className="settings-subpanel"><h3>Runner Backends</h3><div className="settings-list">{backends.map((backend) => <div className="settings-list-item static" key={backend.key}><div><strong>{backend.label}</strong><span>{backend.providerType} / {backend.state} / {formatTimestamp(backend.lastCheckedAt)}</span></div><button className="secondary-btn" disabled={busy} onClick={() => void runBackendAction(backend)} type="button">{backend.actionLabel}</button></div>)}</div></div>
        <div className="settings-subpanel">
          <h3>{editingIntegration ? "Edit Integration" : "Create Integration"}</h3>
          {type === "jira" ? (
            <small className="settings-field-help">
              Connect with your Atlassian <strong>email + API token</strong> (same as the old form). FlowPilot stores
              the token in the runner keyring and uses it for Atlassian <strong>Rovo MCP</strong> (
              <code>Basic</code> → <code>mcp.atlassian.com/v1/mcp</code>) after Configure Providers. Your org admin may
              need to enable API token auth for Rovo MCP.{" "}
              <a href="https://support.atlassian.com/security-and-access-policies/docs/control-atlassian-rovo-mcp-server-settings/#Configure-authentication" rel="noreferrer" target="_blank">
                Enable Rovo MCP API token (admin)
              </a>
              {" · "}
              <a href="https://id.atlassian.com/manage-profile/security/api-tokens?autofillToken&expiryDays=max&appId=mcp&selectedScopes=all" rel="noreferrer" target="_blank">
                Create MCP-scoped API token
              </a>
              {" · "}
              <a href="https://support.atlassian.com/atlassian-rovo-mcp-server/docs/configuring-authentication-via-api-token/" rel="noreferrer" target="_blank">
                Auth via API token docs
              </a>
            </small>
          ) : null}
          <div className="settings-grid">
            {supportsGlobalScope ? (
              <label className="settings-field">
                <span>Owner scope</span>
                <select disabled={Boolean(editingIntegration)} value={ownerScope} onChange={(event) => setOwnerScope(event.target.value as IntegrationOwnerScope)}>
                  <option value="global">Workspace global</option>
                  <option value="project">Project scoped</option>
                </select>
              </label>
            ) : null}
            {supportsGlobalScope && ownerScope === "project" ? (
              <label className="settings-field">
                <span>Owner project</span>
                <select disabled={Boolean(editingIntegration)} value={projectId} onChange={(event) => setProjectId(event.target.value)}>
                  {projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}
                </select>
              </label>
            ) : null}
            {mode === "all" ? <label className="settings-field"><span>Type</span><select disabled={Boolean(editingIntegration)} value={type} onChange={(event) => changeType(event.target.value as IntegrationType)}>{types.map((option) => <option key={option} value={option}>{option}</option>)}</select></label> : null}
            <label className="settings-field settings-field-full"><span>Label</span><input value={label} onChange={(event) => setLabel(event.target.value)} /></label>
            {providerFields[type].map((field) => (
              // Include type in the key so React does not reuse an input DOM
              // node (and its displayed value) when switching MCP pages.
              <div className="settings-field" key={`${type}:${field.key}`}>
                <span>{field.label}{field.required ? " *" : ""}</span>
                <input type={field.type ?? "text"} value={config[field.key] ?? ""} onChange={(event) => setConfig((current) => ({ ...current, [field.key]: event.target.value }))} />
                {renderFieldGuide(field)}
              </div>
            ))}
            {type === "telegram" ? (
              <label className="settings-checkbox settings-field-full">
                <input
                  checked={telegramAutoApprove}
                  onChange={(event) => setTelegramAutoApprove(event.target.checked)}
                  type="checkbox"
                />
                <span>
                  Auto-approve sends (CLI / dev)
                  <small className="settings-field-help">
                    When checked, <code>send_message</code> without a FlowPilot run scope (e.g. Codex CLI)
                    sends immediately. Unchecked (default) refuses those calls with{" "}
                    <code>MCP_TOOL_APPROVAL_REQUIRED</code>. In-app FlowPilot runs still use the Pending
                    Approvals queue when run/step scope is present. Flag is stored in the{" "}
                    <strong>runner keyring</strong> with the bot credential — not in Supabase secrets.
                  </small>
                </span>
              </label>
            ) : null}
          </div>
          <div className="settings-actions">
            <button className="primary-btn" disabled={busy || (supportsGlobalScope && ownerScope === "project" && !projectId)} onClick={() => void createIntegration()} type="button">{editingIntegration ? "Save and Reconnect" : "Create Integration"}</button>
            {editingIntegration ? <button className="secondary-btn" disabled={busy} onClick={() => resetEditor()} type="button">Cancel Edit</button> : null}
          </div></div>
      </div>
      <div className="settings-subpanel">
        <h3>Existing Integrations</h3>
        <div className="settings-list">
          {integrations.map((integration) => {
            const autoApproveOn =
              integration.type === "telegram" &&
              ((integration.configEncrypted as Record<string, unknown>).autoApprove === true ||
                (integration.configEncrypted as Record<string, unknown>).autoApprove === "true");
            return (
              <div className="settings-list-item static" key={integration.id}>
                <div>
                  <strong>{integration.label}</strong>
                  <span>
                    {integration.type} / {integration.status} /{" "}
                    {integration.projectId ? `project ${integration.projectId}` : "workspace global"}
                    {integration.type === "telegram"
                      ? autoApproveOn
                        ? " / auto-approve ON"
                        : " / auto-approve OFF"
                      : ""}
                  </span>
                </div>
                <div className="settings-actions">
                  <button
                    className="secondary-btn"
                    disabled={busy}
                    onClick={() => beginEditIntegration(integration)}
                    type="button"
                  >
                    {integration.type === "jira" ? "Edit / Rotate token" : "Edit"}
                  </button>
                  {integration.type === "jira" ||
                  integration.type === "google_drive" ||
                  integration.type === "firebase" ||
                  integration.type === "telegram" ? (
                    <button
                      className="secondary-btn"
                      disabled={busy}
                      onClick={() => void testIntegration(integration)}
                      type="button"
                    >
                      Test
                    </button>
                  ) : null}
                  {integration.type === "telegram" ? (
                    <button
                      className="secondary-btn"
                      disabled={busy}
                      onClick={() => void setTelegramAutoApproveOnIntegration(integration, !autoApproveOn)}
                      type="button"
                      title={
                        autoApproveOn
                          ? "Disable auto-approve (CLI sends will require this flag again)"
                          : "Enable auto-approve so Codex CLI can send without Pending Approvals"
                      }
                    >
                      {autoApproveOn ? "Disable auto-approve" : "Enable auto-approve"}
                    </button>
                  ) : null}
                  <button
                    className="secondary-btn danger-btn"
                    disabled={busy}
                    onClick={() => void deleteIntegration(integration)}
                    type="button"
                  >
                    Delete
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      </div>
      {ensurePath ? (
        <div className="settings-subpanel">
          <h3>Configure Providers</h3>
          <small className="settings-field-help">
            {type === "jira"
              ? "Writes mcpServers.jira (Atlassian Rovo remote MCP) into every discovered AI provider account (Claude, Codex, Gemini, Grok). Uses the connected email+API token as Basic auth against mcp.atlassian.com/v1/mcp. Live Claude/Grok turns also merge this server automatically when connected."
              : `Writes mcpServers.${type} into every discovered AI provider account (Claude, Codex, Gemini, Grok) so each can launch the ${type === "firebase" ? "firebase-tools" : "flowpilot telegram-mcp"} server.`}
          </small>
          {type === "jira" ? (
            <small className="settings-field-help">
              Admin must allow API token auth for Rovo MCP:{" "}
              <a href="https://support.atlassian.com/security-and-access-policies/docs/control-atlassian-rovo-mcp-server-settings/#Configure-authentication" rel="noreferrer" target="_blank">
                Control Rovo MCP authentication
              </a>
            </small>
          ) : null}
          {configureMessage ? <div className="settings-feedback">{configureMessage}</div> : null}
          <div className="settings-list">
            {providerAccounts.length === 0 ? (
              <div className="settings-list-empty">No AI provider accounts found.</div>
            ) : (
              providerAccounts.map((account) => (
                <div className="settings-list-item static" key={`${account.provider_key}:${account.home_path}`}>
                  <div>
                    <strong>{PROVIDER_LABELS[account.provider_key] ?? account.provider_key}</strong>
                    <span>{account.display_name || account.home_path}</span>
                  </div>
                </div>
              ))
            )}
          </div>
          <div className="settings-actions">
            <button
              className="primary-btn"
              disabled={busy || providerAccounts.length === 0}
              onClick={() => void configureProviders()}
              type="button"
            >
              Configure Providers
            </button>
          </div>
        </div>
      ) : null}
      {(mode === "telegram" || mode === "all") && integrations.some((integration) => integration.type === "telegram") ? (
        <div className="settings-subpanel">
          <h3>Pending Telegram Approvals</h3>
          <small className="settings-field-help">
            Per-send queue for <strong>in-app FlowPilot runs</strong> that have run/step scope (Task-233). Stored
            under the workspace as <code>.flowpilot/telegram-proxy-approvals.json</code>. Approve → AI retries the
            same <code>send_message</code> and it sends for real; Reject blocks it. Separate from the integration{" "}
            <strong>Auto-approve</strong> flag (keyring), which only covers CLI / no-scope calls.
          </small>
          <div className="settings-list">
            {telegramApprovals.length === 0 ? (
              <div className="settings-list-empty">No pending Telegram sends.</div>
            ) : (
              telegramApprovals.map((approval) => (
                <div className="settings-list-item static" key={approval.id}>
                  <div>
                    <strong>To chat {approval.chatId}</strong>
                    <span>{approval.text}</span>
                    <span>requested {formatTimestamp(approval.requestedAt)}</span>
                  </div>
                  <div className="settings-actions">
                    <button className="primary-btn" disabled={busy} onClick={() => void decideTelegramApproval(approval.id, "approved")} type="button">
                      Approve
                    </button>
                    <button className="secondary-btn" disabled={busy} onClick={() => void decideTelegramApproval(approval.id, "rejected")} type="button">
                      Reject
                    </button>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      ) : null}
      {errorModal ? (
        <div className="settings-modal-backdrop" role="presentation" onClick={() => setErrorModal(null)}>
          <div className="settings-modal mcp-error-modal" role="alertdialog" aria-modal="true" aria-label={errorModal.title} onClick={(event) => event.stopPropagation()}>
            <div className="settings-panel-head">
              <div>
                <div className="settings-eyebrow">Error</div>
                <h2>{errorModal.title}</h2>
              </div>
            </div>
            <p className="mcp-error-modal-body">{errorModal.body}</p>
            <div className="settings-actions">
              <button className="primary-btn" autoFocus onClick={() => setErrorModal(null)} type="button">Close</button>
            </div>
          </div>
        </div>
      ) : null}
    </section>
  );
}
