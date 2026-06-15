import { useEffect, useMemo, useState } from "react";
import { RUNNER_URL } from "@/config";

interface GoogleDriveArtifactSyncStatus {
  status: string;
  source: string;
  configured: boolean;
  clientId?: string;
  redirectUri?: string;
  hasClientSecret: boolean;
  hasPickerApiKey: boolean;
  missingFields?: string[];
}

interface GoogleDriveMcpStatus {
  status: string;
  configured: boolean;
  proxyMcpEnabled: boolean;
  credentialPath?: string;
  tokenPath?: string;
  accountId?: string;
  accountEmail?: string;
  accountSelectionRequired?: boolean;
  grantedScopes?: string[];
  missingScopes?: string[];
  accountReady: boolean;
  mcpReadReady: boolean;
  mcpWriteReady: boolean;
  reconnectRequired?: boolean;
}

interface GoogleDriveAccountStatus {
  accountId: string;
  accountEmail?: string;
  oauthClientId?: string;
  grantedScopes?: string[];
  missingScopes?: string[];
  status: string;
  projectCount: number;
  mcpReadReady: boolean;
  mcpWriteReady: boolean;
  connectedAt?: string;
  updatedAt?: string;
  lastError?: string;
}

interface GoogleDriveMcpProviderConfigStatus {
  providerKey: string;
  serverName: string;
  status: string;
  changed: boolean;
  configPath: string;
  lastError?: string;
}

interface GoogleDriveWorkspaceConfigResponse {
  artifactSync: GoogleDriveArtifactSyncStatus;
  mcp: GoogleDriveMcpStatus;
  accounts?: GoogleDriveAccountStatus[];
  providerConfigs?: GoogleDriveMcpProviderConfigStatus[];
  runnerReachable: boolean;
  lastError?: string | null;
}

interface GoogleDriveValidationCheck {
  key: string;
  status: "passed" | "failed" | "skipped";
  message: string;
}

interface GoogleDriveValidationResult {
  valid: boolean;
  checks: GoogleDriveValidationCheck[];
}

const DEFAULT_REDIRECT_URI = "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback";

function runnerFetch(path: string, init?: RequestInit): Promise<Response> {
  return fetch(new URL(path, RUNNER_URL).toString(), { cache: "no-store", ...init });
}

async function readError(response: Response): Promise<string> {
  const text = await response.text().catch(() => "");
  if (!text) return `Request failed with status ${response.status}.`;
  try {
    const payload = JSON.parse(text) as { error?: string };
    return payload.error || text;
  } catch {
    return text;
  }
}

function isStepConfigured(value: string | undefined) {
  return value === "configured" || value === "online" || value === "connected" || value === "ready";
}

function artifactSyncStepStatus(sync?: GoogleDriveArtifactSyncStatus | null) {
  if (!sync) return "not_started";
  return sync.clientId?.trim() && sync.redirectUri?.trim() && sync.hasClientSecret
    ? "configured"
    : "needs_input";
}

function providerSetupStatus(status: GoogleDriveWorkspaceConfigResponse | null) {
  const configs = status?.providerConfigs ?? [];
  if (configs.length === 0) return "not_started";
  if (configs.some((c) => c.status === "failed")) return "failed";
  if (configs.every((c) => c.status === "configured")) return "configured";
  return "needs_input";
}

function buildStepSummary(status: GoogleDriveWorkspaceConfigResponse | null, s1: boolean, s2: boolean, s3: boolean) {
  const configured = Boolean(
    status?.artifactSync.clientId?.trim() &&
      status?.artifactSync.redirectUri?.trim() &&
      status?.artifactSync.hasClientSecret,
  );
  return {
    project: configured || s1 ? "configured" : "not_started",
    consent: configured || s2 ? "configured" : "not_started",
    apis: configured || s3 ? "configured" : "not_started",
  };
}

function GdriveBadge({ value }: { value: string }) {
  let cls = "gdrive-badge";
  if (value === "configured" || value === "online" || value === "connected" || value === "ready") cls += " gdrive-badge-ok";
  else if (value === "needs_input" || value === "needs_auth" || value === "reconnect_required" || value === "warning" || value === "config_stale") cls += " gdrive-badge-warn";
  else if (value === "failed") cls += " gdrive-badge-err";
  return <span className={cls}>{value}</span>;
}

function StepCircle({ index, done, active, status }: { index: number; done: boolean; active: boolean; status: string }) {
  let cls = "gdrive-step-circle";
  if (done) cls += " done";
  else if (status === "failed") cls += " err";
  else if (status === "needs_input" || status === "reconnect_required" || status === "needs_auth" || status === "warning") cls += " warn";
  else if (active) cls += " active";
  return <div className={cls}>{done ? "✓" : index}</div>;
}

function CopyBtn({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      className="ghost-btn gdrive-copy-btn"
      type="button"
      onClick={() => {
        void navigator.clipboard.writeText(value).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        });
      }}
    >
      {copied ? "Copied" : "Copy"}
    </button>
  );
}

function CodeRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="gdrive-code-row">
      <div className="gdrive-code-label">{label}</div>
      <div className="gdrive-code-block">
        <code className="gdrive-code">{value}</code>
        <CopyBtn value={value} />
      </div>
    </div>
  );
}

function GuideSteps({ items, links }: { items: string[]; links?: { label: string; href: string }[] }) {
  return (
    <details className="gdrive-guide">
      <summary className="gdrive-guide-toggle">Show detailed steps</summary>
      <ol className="gdrive-guide-list">
        {items.map((item, i) => (
          <li key={item}><strong>{i + 1}.</strong> {item}</li>
        ))}
      </ol>
      {links?.map((link) => (
        <a key={link.href} className="gdrive-guide-link" href={link.href} target="_blank" rel="noreferrer">
          {link.label} ↗
        </a>
      ))}
    </details>
  );
}

export function GoogleDriveSettings(): React.ReactElement {
  const [status, setStatus] = useState<GoogleDriveWorkspaceConfigResponse | null>(null);
  const [activeStep, setActiveStep] = useState(1);
  const [artifactForm, setArtifactForm] = useState({ clientId: "", clientSecret: "", redirectUri: DEFAULT_REDIRECT_URI });
  const [pickerApiKey, setPickerApiKey] = useState("");
  const [mcpAccountId, setMcpAccountId] = useState("");
  const [showSecret, setShowSecret] = useState(false);
  const [showPickerKey, setShowPickerKey] = useState(false);
  const [step1Done, setStep1Done] = useState(() => localStorage.getItem("flowpilot_gdrive_step1_done") === "true");
  const [step2Done, setStep2Done] = useState(() => localStorage.getItem("flowpilot_gdrive_step2_done") === "true");
  const [step3Done, setStep3Done] = useState(() => localStorage.getItem("flowpilot_gdrive_step3_done") === "true");
  const [expandedAccounts, setExpandedAccounts] = useState<Record<string, boolean>>({});
  const [busyAction, setBusyAction] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [validation, setValidation] = useState<GoogleDriveValidationResult | null>(null);
  const [showValidation, setShowValidation] = useState(false);

  const applyStatus = (next: GoogleDriveWorkspaceConfigResponse) => {
    setStatus(next);
    setMcpAccountId(
      next.mcp.accountId?.trim() ||
        (next.accounts?.length === 1 ? next.accounts[0].accountId?.trim() ?? "" : "") ||
        "",
    );
  };

  const loadStatus = async () => {
    try {
      const res = await runnerFetch("/google-drive-config");
      if (res.ok) {
        const payload = (await res.json()) as GoogleDriveWorkspaceConfigResponse;
        applyStatus(payload);
        setArtifactForm({
          clientId: payload.artifactSync.clientId ?? "",
          clientSecret: "",
          redirectUri: payload.artifactSync.redirectUri ?? DEFAULT_REDIRECT_URI,
        });

        const s1 = localStorage.getItem("flowpilot_gdrive_step1_done") === "true";
        const s2 = localStorage.getItem("flowpilot_gdrive_step2_done") === "true";
        const s3 = localStorage.getItem("flowpilot_gdrive_step3_done") === "true";
        const sum = buildStepSummary(payload, s1, s2, s3);
        if (!isStepConfigured(sum.project)) setActiveStep(1);
        else if (!isStepConfigured(sum.consent)) setActiveStep(2);
        else if (!isStepConfigured(sum.apis)) setActiveStep(3);
        else if (!isStepConfigured(artifactSyncStepStatus(payload.artifactSync))) setActiveStep(4);
        else if (!payload.artifactSync.hasPickerApiKey) setActiveStep(5);
        else if (!isStepConfigured(payload.mcp.status)) setActiveStep(6);
        else setActiveStep(7);
        return payload;
      }
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to load Google Drive status.");
    }
    return null;
  };

  useEffect(() => {
    void loadStatus();
  }, []);

  useEffect(() => {
    setExpandedAccounts({});
  }, [activeStep]);

  useEffect(() => {
    const handler = (event: MessageEvent) => {
      if (event.data?.type === "flowpilot-google-drive-account-connected") {
        void loadStatus().then(() => setMessage("Google Drive account status refreshed."));
      }
    };
    window.addEventListener("message", handler);
    return () => window.removeEventListener("message", handler);
  }, []);

  const stepSummary = useMemo(() => buildStepSummary(status, step1Done, step2Done, step3Done), [status, step1Done, step2Done, step3Done]);
  const proxyAccounts = status?.accounts ?? [];
  const selectedAccount = proxyAccounts.find((a) => a.accountId === mcpAccountId) ?? null;

  const saveArtifactSync = async () => {
    setBusyAction("save-artifact");
    setMessage(null);
    try {
      const res = await runnerFetch("/google-drive-config", {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ clientId: artifactForm.clientId, clientSecret: artifactForm.clientSecret, redirectUri: artifactForm.redirectUri }),
      });
      if (!res.ok) throw new Error(await readError(res));
      await loadStatus();
      setArtifactForm((prev) => ({ ...prev, clientSecret: "" }));
      setMessage("Artifact sync configuration saved.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to save artifact sync config.");
    } finally {
      setBusyAction(null);
    }
  };

  const savePickerApiKey = async () => {
    setBusyAction("save-picker");
    setMessage(null);
    try {
      const res = await runnerFetch("/google-drive-config", {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ pickerApiKey }),
      });
      if (!res.ok) throw new Error(await readError(res));
      await loadStatus();
      setPickerApiKey("");
      setMessage("Google Picker API key saved.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to save Picker API key.");
    } finally {
      setBusyAction(null);
    }
  };

  const saveProxyAccount = async () => {
    setBusyAction("save-proxy-account");
    setMessage(null);
    try {
      const res = await runnerFetch("/google-drive-config", {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ mcpAccountId: mcpAccountId.trim() }),
      });
      if (!res.ok) throw new Error(await readError(res));
      await loadStatus();
      setMessage("Google account selection saved.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to save Google account selection.");
    } finally {
      setBusyAction(null);
    }
  };

  const startAccountConnect = async (accountId?: string) => {
    setBusyAction(accountId ? `reconnect:${accountId}` : "connect-account");
    setMessage(null);
    try {
      const res = await runnerFetch("/google-drive-config/accounts/connect-sessions", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(accountId ? { accountId } : {}),
      });
      const payload = await res.json().catch(() => ({})) as { connectUrl?: string; error?: string };
      if (!res.ok) throw new Error(payload.error ?? "Unable to start account connection.");
      if (typeof payload.connectUrl === "string" && payload.connectUrl.trim()) {
        window.open(payload.connectUrl, "_blank", "width=980,height=820");
      }
      setMessage(accountId ? "Reconnect started — complete in the popup." : "Connect started — complete in the popup.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to start account connection.");
    } finally {
      setBusyAction(null);
    }
  };

  const disconnectAccount = async (accountId: string) => {
    setBusyAction(`disconnect:${accountId}`);
    setMessage(null);
    try {
      const res = await runnerFetch(`/google-drive-config/accounts/${encodeURIComponent(accountId)}`, { method: "DELETE" });
      if (!res.ok && res.status !== 204) throw new Error(await readError(res));
      await loadStatus();
      if (mcpAccountId === accountId) setMcpAccountId("");
      setMessage("Google Drive account disconnected.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to disconnect account.");
    } finally {
      setBusyAction(null);
    }
  };

  const validateSetup = async () => {
    setBusyAction("validate");
    setMessage(null);
    try {
      const payload: Record<string, string> = {
        clientId: artifactForm.clientId,
        clientSecret: artifactForm.clientSecret,
        redirectUri: artifactForm.redirectUri,
        pickerApiKey,
      };
      if (mcpAccountId.trim()) payload.mcpAccountId = mcpAccountId.trim();
      const res = await runnerFetch("/google-drive-config/validate", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(payload),
      });
      const result = await res.json() as GoogleDriveValidationResult;
      setValidation(result);
      setShowValidation(true);
      setMessage(result.valid ? "Google Drive checks passed." : "Fix the failed checks before saving.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to validate Google Drive config.");
    } finally {
      setBusyAction(null);
    }
  };

  const resetConfig = async () => {
    setBusyAction("reset");
    setMessage(null);
    try {
      const res = await runnerFetch("/google-drive-config", { method: "DELETE" });
      if (!res.ok) throw new Error(await readError(res));
      localStorage.removeItem("flowpilot_gdrive_step1_done");
      localStorage.removeItem("flowpilot_gdrive_step2_done");
      localStorage.removeItem("flowpilot_gdrive_step3_done");
      setStep1Done(false);
      setStep2Done(false);
      setStep3Done(false);
      setValidation(null);
      await loadStatus();
      setMessage("Google Drive config reset.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to reset Google Drive config.");
    } finally {
      setBusyAction(null);
    }
  };

  const confirmStep1 = () => {
    setStep1Done(true);
    localStorage.setItem("flowpilot_gdrive_step1_done", "true");
    setActiveStep(2);
  };
  const confirmStep2 = () => {
    setStep2Done(true);
    localStorage.setItem("flowpilot_gdrive_step2_done", "true");
    setActiveStep(3);
  };
  const confirmStep3 = () => {
    setStep3Done(true);
    localStorage.setItem("flowpilot_gdrive_step3_done", "true");
    setActiveStep(4);
  };

  const steps = [
    { index: 1, label: "Create project", status: stepSummary.project },
    { index: 2, label: "OAuth consent", status: stepSummary.consent },
    { index: 3, label: "Enable APIs", status: stepSummary.apis },
    { index: 4, label: "OAuth client", status: artifactSyncStepStatus(status?.artifactSync) },
    { index: 5, label: "Picker API key", status: status?.artifactSync.hasPickerApiKey ? "configured" : "needs_input" },
    { index: 6, label: "Google account", status: status?.mcp.status ?? "not_started" },
    { index: 7, label: "Proxy MCP", status: providerSetupStatus(status) },
  ];

  return (
    <section className="settings-panel">
      <div className="settings-panel-head">
        <div>
          <div className="settings-eyebrow">Settings</div>
          <h2>Google Drive Setup</h2>
          <p>Configure Google Cloud credentials for artifact sync and the proxy MCP.</p>
        </div>
        <div className="settings-actions" style={{ marginTop: 0 }}>
          <button className="secondary-btn" disabled={busyAction === "validate"} onClick={() => void validateSetup()} type="button">
            {busyAction === "validate" ? "Validating..." : "Validate setup"}
          </button>
          <button className="secondary-btn" disabled={busyAction === "reset"} onClick={() => void resetConfig()} type="button">
            {busyAction === "reset" ? "Resetting..." : "Reset config"}
          </button>
        </div>
      </div>

      {/* Status overview */}
      <div className="settings-subpanel">
        <h3>Current Status</h3>
        <div className="gdrive-status-grid">
          <div className="gdrive-status-row">
            <span>Artifact sync</span>
            <GdriveBadge value={status?.artifactSync.status ?? "loading"} />
            <span className="gdrive-status-detail">source: {status?.artifactSync.source ?? "—"}</span>
          </div>
          <div className="gdrive-status-row">
            <span>Proxy MCP</span>
            <GdriveBadge value={status?.mcp.status ?? "loading"} />
            <span className="gdrive-status-detail">
              {status?.mcp.accountEmail ? `account: ${status.mcp.accountEmail}` : status?.mcp.accountSelectionRequired ? "select account" : "—"}
            </span>
          </div>
          <div className="gdrive-status-row">
            <span>Runner</span>
            <GdriveBadge value={status?.runnerReachable ? "online" : "offline"} />
          </div>
        </div>
        {status?.mcp.credentialPath || status?.mcp.tokenPath ? (
          <div className="gdrive-path-info">
            {status.mcp.credentialPath ? <p>MCP credentials: <code>{status.mcp.credentialPath}</code></p> : null}
            {status.mcp.tokenPath ? <p>MCP tokens: <code>{status.mcp.tokenPath}</code></p> : null}
          </div>
        ) : null}
      </div>

      {message ? (
        <div className={`settings-feedback${status?.lastError || (validation && !validation.valid) ? " error" : ""}`}>
          {message}
          {validation && !validation.valid ? (
            <button className="ghost-btn" style={{ marginLeft: 8 }} onClick={() => setShowValidation(true)} type="button">
              View checks
            </button>
          ) : null}
        </div>
      ) : null}
      {status?.lastError ? (
        <div className="settings-feedback error" style={{ marginTop: 8 }}>
          {status.lastError}
        </div>
      ) : null}

      {/* Stepper */}
      <div className="settings-subpanel">
        <div className="gdrive-stepper">
          {steps.map((step) => {
            const done = isStepConfigured(step.status);
            const active = activeStep === step.index;
            return (
              <button
                key={step.index}
                className={`gdrive-step${active ? " active" : ""}${done ? " done" : ""}`}
                onClick={() => setActiveStep(step.index)}
                type="button"
              >
                <StepCircle index={step.index} done={done} active={active} status={step.status} />
                <span className="gdrive-step-label">{step.label}</span>
              </button>
            );
          })}
        </div>
      </div>

      {/* Step content */}
      {activeStep === 1 && (
        <div className="settings-subpanel">
          <div className="gdrive-step-header">
            <div>
              <div className="settings-eyebrow">Step 1</div>
              <h3 style={{ marginTop: 4 }}>Create Google Cloud project</h3>
              <p className="gdrive-step-copy">Use one Google Cloud project for this setup and keep it active before creating clients or keys.</p>
            </div>
            <GdriveBadge value={stepSummary.project} />
          </div>
          <GuideSteps items={[
            "Open Google Cloud Console.",
            "Use the top project selector to create a project (e.g. FlowPilot Local).",
            "Make sure that project is active before creating OAuth clients or API keys.",
          ]} />
          <div className="settings-actions">
            <button className="primary-btn" onClick={confirmStep1} type="button">Confirm done</button>
            <button className="secondary-btn" onClick={() => setActiveStep(2)} type="button">Next step</button>
          </div>
        </div>
      )}

      {activeStep === 2 && (
        <div className="settings-subpanel">
          <div className="gdrive-step-header">
            <div>
              <div className="settings-eyebrow">Step 2</div>
              <h3 style={{ marginTop: 4 }}>Configure OAuth consent screen</h3>
              <p className="gdrive-step-copy">Use External, add test users, switch to Production when testing is done.</p>
            </div>
            <GdriveBadge value={stepSummary.consent} />
          </div>
          <GuideSteps
            items={[
              "Open Google Auth Platform > Audience.",
              "Choose External for personal testing.",
              "Add every Google account you will use as a test user.",
              "Move to Production when you want long-lived refresh tokens.",
            ]}
            links={[{ label: "OAuth consent guide", href: "https://developers.google.com/workspace/guides/configure-oauth-consent" }]}
          />
          <div className="settings-actions">
            <button className="primary-btn" onClick={confirmStep2} type="button">Confirm done</button>
            <button className="secondary-btn" onClick={() => setActiveStep(3)} type="button">Next step</button>
          </div>
        </div>
      )}

      {activeStep === 3 && (
        <div className="settings-subpanel">
          <div className="gdrive-step-header">
            <div>
              <div className="settings-eyebrow">Step 3</div>
              <h3 style={{ marginTop: 4 }}>Enable APIs</h3>
              <p className="gdrive-step-copy">Enable Google Drive API, Google Picker API, and optionally Docs/Sheets/Slides for broader MCP support.</p>
            </div>
            <GdriveBadge value={stepSummary.apis} />
          </div>
          <GuideSteps items={[
            "Open APIs & Services > Library.",
            "Enable Google Drive API.",
            "Enable Google Picker API.",
            "Optionally enable Google Docs, Sheets, and Slides APIs for broader MCP coverage.",
          ]} />
          <div className="settings-actions">
            <button className="primary-btn" onClick={confirmStep3} type="button">Confirm done</button>
            <button className="secondary-btn" onClick={() => setActiveStep(4)} type="button">Next step</button>
          </div>
        </div>
      )}

      {activeStep === 4 && (
        <div className="settings-subpanel">
          <div className="gdrive-step-header">
            <div>
              <div className="settings-eyebrow">Step 4</div>
              <h3 style={{ marginTop: 4 }}>Create Web OAuth client</h3>
              <p className="gdrive-step-copy">Save Web OAuth client credentials locally so FlowPilot can use them for artifact sync and proxy MCP.</p>
            </div>
            <GdriveBadge value={artifactSyncStepStatus(status?.artifactSync)} />
          </div>
          <GuideSteps
            items={[
              "Open Google Auth Platform > Clients.",
              "Click Create client > Web application.",
              "Name it FlowPilot Google Drive Local.",
              "Add the redirect URI shown below.",
              "Copy Client ID and Client Secret into the form.",
            ]}
            links={[{ label: "Create credentials guide", href: "https://developers.google.com/workspace/guides/create-credentials" }]}
          />
          <CodeRow label="Required redirect URI" value={DEFAULT_REDIRECT_URI} />
          <div className="settings-grid" style={{ marginTop: 16 }}>
            <label className="settings-field">
              <span>Client ID</span>
              <input
                placeholder="Insert Client ID here"
                value={artifactForm.clientId}
                onChange={(e) => setArtifactForm((prev) => ({ ...prev, clientId: e.target.value }))}
                autoComplete="off"
              />
            </label>
            <label className="settings-field">
              <span>Client Secret <em style={{ fontStyle: "normal", color: "var(--text-dim)" }}>(leave blank to keep stored)</em></span>
              <div className="gdrive-secret-row">
                <input
                  placeholder="Google OAuth client secret"
                  type={showSecret ? "text" : "password"}
                  value={artifactForm.clientSecret}
                  onChange={(e) => setArtifactForm((prev) => ({ ...prev, clientSecret: e.target.value }))}
                  autoComplete="new-password"
                  style={{ flex: 1 }}
                />
                <button className="ghost-btn" onClick={() => setShowSecret((v) => !v)} type="button" style={{ padding: "0 10px" }}>
                  {showSecret ? "Hide" : "Show"}
                </button>
              </div>
            </label>
            <label className="settings-field settings-field-full">
              <span>Redirect URI</span>
              <div className="gdrive-secret-row">
                <input
                  value={artifactForm.redirectUri}
                  onChange={(e) => setArtifactForm((prev) => ({ ...prev, redirectUri: e.target.value }))}
                  style={{ flex: 1 }}
                />
                <CopyBtn value={artifactForm.redirectUri} />
              </div>
            </label>
          </div>
          <div className="settings-actions">
            <button className="primary-btn" disabled={busyAction === "save-artifact" || !artifactForm.clientId.trim()} onClick={() => void saveArtifactSync()} type="button">
              {busyAction === "save-artifact" ? "Saving..." : "Save artifact sync"}
            </button>
            <button className="secondary-btn" onClick={() => setActiveStep(5)} type="button">Next step</button>
          </div>
        </div>
      )}

      {activeStep === 5 && (
        <div className="settings-subpanel">
          <div className="gdrive-step-header">
            <div>
              <div className="settings-eyebrow">Step 5</div>
              <h3 style={{ marginTop: 4 }}>Create Picker API key</h3>
              <p className="gdrive-step-copy">Save the browser-side Picker API key separately from the OAuth client secret.</p>
            </div>
            <GdriveBadge value={status?.artifactSync.hasPickerApiKey ? "configured" : "needs_input"} />
          </div>
          <GuideSteps items={[
            "Open APIs & Services > Credentials.",
            "Click Create Credentials > API key.",
            "Name it FlowPilot Picker API Key.",
            "Restrict the key to Google Picker API.",
            "For local MVP testing, keep Application restrictions as None.",
            "Copy the key into the field below.",
          ]} />
          <div className="settings-grid" style={{ marginTop: 16 }}>
            <label className="settings-field settings-field-full">
              <span>Picker API key <em style={{ fontStyle: "normal", color: "var(--text-dim)" }}>(leave blank to keep stored)</em></span>
              <div className="gdrive-secret-row">
                <input
                  placeholder="Google Picker API key"
                  type={showPickerKey ? "text" : "password"}
                  value={pickerApiKey}
                  onChange={(e) => setPickerApiKey(e.target.value)}
                  autoComplete="new-password"
                  style={{ flex: 1 }}
                />
                <button className="ghost-btn" onClick={() => setShowPickerKey((v) => !v)} type="button" style={{ padding: "0 10px" }}>
                  {showPickerKey ? "Hide" : "Show"}
                </button>
              </div>
            </label>
          </div>
          <div className="settings-actions">
            <button className="primary-btn" disabled={busyAction === "save-picker" || !pickerApiKey.trim()} onClick={() => void savePickerApiKey()} type="button">
              {busyAction === "save-picker" ? "Saving..." : "Save picker key"}
            </button>
            <button className="secondary-btn" onClick={() => setActiveStep(6)} type="button">Next step</button>
          </div>
        </div>
      )}

      {activeStep === 6 && (
        <div className="settings-subpanel">
          <div className="gdrive-step-header">
            <div>
              <div className="settings-eyebrow">Step 6</div>
              <h3 style={{ marginTop: 4 }}>Select Google account for proxy MCP</h3>
              <p className="gdrive-step-copy">Connect at least one Google account. Project artifact folders are configured separately per project.</p>
            </div>
            <GdriveBadge value={status?.mcp.status ?? "not_started"} />
          </div>
          <div className="settings-actions" style={{ marginTop: 12 }}>
            <button className="primary-btn" disabled={busyAction !== null} onClick={() => void startAccountConnect()} type="button">
              {busyAction === "connect-account" ? "Opening..." : "Connect Google account"}
            </button>
          </div>
          <div className="settings-grid" style={{ marginTop: 16 }}>
            <label className="settings-field">
              <span>Active account ({proxyAccounts.length > 0 ? `${proxyAccounts.length} available` : "none connected"})</span>
              <select value={mcpAccountId} onChange={(e) => setMcpAccountId(e.target.value)}>
                <option value="">Select a connected Google account</option>
                {proxyAccounts.map((acc) => (
                  <option key={acc.accountId} value={acc.accountId}>
                    {acc.accountEmail ? `${acc.accountEmail} (${acc.accountId})` : acc.accountId}
                    {acc.projectCount > 0 ? ` — ${acc.projectCount} project${acc.projectCount === 1 ? "" : "s"}` : ""}
                  </option>
                ))}
              </select>
            </label>
          </div>
          {selectedAccount ? (
            <div className="settings-card" style={{ marginTop: 12 }}>
              <div className="settings-card-head">
                <strong>Selected account capabilities</strong>
                <GdriveBadge value={selectedAccount.status} />
              </div>
              <div className="gdrive-badge-row">
                <GdriveBadge value={selectedAccount.mcpReadReady ? "proxy read ready" : "proxy read missing scope"} />
                <GdriveBadge value={selectedAccount.mcpWriteReady ? "artifact write ready" : "artifact write missing scope"} />
              </div>
              <p style={{ marginTop: 8, fontSize: 12, color: "var(--text-dim)" }}>
                Scopes: {selectedAccount.grantedScopes?.join(", ") || "none recorded"}
              </p>
            </div>
          ) : null}
          <div className="settings-actions">
            <button className="primary-btn" disabled={busyAction === "save-proxy-account" || !mcpAccountId.trim()} onClick={() => void saveProxyAccount()} type="button">
              {busyAction === "save-proxy-account" ? "Saving..." : "Save account selection"}
            </button>
          </div>
          {/* Account list */}
          {proxyAccounts.length === 0 ? (
            <div className="settings-empty" style={{ marginTop: 16 }}>
              No Google accounts connected yet. Click Connect Google account above.
            </div>
          ) : (
            <div className="settings-list" style={{ marginTop: 16 }}>
              {proxyAccounts.map((acc) => {
                const isExpanded = expandedAccounts[acc.accountId] ?? false;
                const savedSelection = acc.accountId === status?.mcp.accountId;
                return (
                  <div
                    key={acc.accountId}
                    className={`settings-list-item static${savedSelection ? " active" : ""}`}
                    style={{ flexDirection: "column", alignItems: "stretch" }}
                  >
                    <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <strong style={{ display: "block" }}>
                          {acc.accountEmail || acc.accountId}
                          {savedSelection ? <span className="gdrive-badge gdrive-badge-ok" style={{ marginLeft: 8 }}>saved</span> : null}
                        </strong>
                        <span>
                          {acc.accountId}
                          {" · "}
                          <GdriveBadge value={acc.status} />
                          {" "}
                          <GdriveBadge value={acc.mcpReadReady ? "read ready" : "read missing scope"} />
                        </span>
                      </div>
                      <button
                        className="ghost-btn"
                        onClick={() => setExpandedAccounts((prev) => ({ ...prev, [acc.accountId]: !prev[acc.accountId] }))}
                        type="button"
                      >
                        {isExpanded ? "▲" : "▼"}
                      </button>
                    </div>
                    {isExpanded && (
                      <div style={{ marginTop: 12, borderTop: "1px solid var(--border)", paddingTop: 12 }}>
                        <p style={{ fontSize: 12, color: "var(--text-dim)", margin: "0 0 4px" }}>Granted scopes: {acc.grantedScopes?.join(", ") || "none"}</p>
                        <p style={{ fontSize: 12, color: "var(--text-dim)", margin: "0 0 4px" }}>Missing scopes: {acc.missingScopes?.join(", ") || "none"}</p>
                        {acc.oauthClientId ? <p style={{ fontSize: 12, color: "var(--text-dim)", margin: "0 0 4px" }}>OAuth client: {acc.oauthClientId}</p> : null}
                        <p style={{ fontSize: 12, color: "var(--text-dim)", margin: "0 0 8px" }}>Connected: {acc.connectedAt || "unknown"}</p>
                        {acc.lastError ? <p style={{ fontSize: 12, color: "var(--err)", marginBottom: 8 }}>{acc.lastError}</p> : null}
                        <div className="settings-actions" style={{ marginTop: 0 }}>
                          <button className="secondary-btn" disabled={busyAction !== null} onClick={() => void startAccountConnect(acc.accountId)} type="button">
                            {busyAction === `reconnect:${acc.accountId}` ? "Opening..." : "Reconnect"}
                          </button>
                          <button className="secondary-btn" disabled={busyAction !== null} onClick={() => void disconnectAccount(acc.accountId)} type="button">
                            {busyAction === `disconnect:${acc.accountId}` ? "Disconnecting..." : "Disconnect"}
                          </button>
                        </div>
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
          <div className="settings-actions" style={{ marginTop: 16 }}>
            <button className="secondary-btn" onClick={() => setActiveStep(7)} type="button">Next step</button>
          </div>
        </div>
      )}

      {activeStep === 7 && (
        <div className="settings-subpanel">
          <div className="gdrive-step-header">
            <div>
              <div className="settings-eyebrow">Step 7</div>
              <h3 style={{ marginTop: 4 }}>Google Drive proxy MCP setup</h3>
              <p className="gdrive-step-copy">Configure Codex, Gemini, and Claude against the FlowPilot proxy MCP.</p>
            </div>
            <GdriveBadge value={providerSetupStatus(status)} />
          </div>
          {(status?.providerConfigs?.length ?? 0) === 0 ? (
            <div className="settings-empty" style={{ marginTop: 12 }}>
              No provider MCP configurations found. Complete steps 4–6 first, then restart the runner to generate provider configs.
            </div>
          ) : (
            <div className="settings-list" style={{ marginTop: 12 }}>
              {status!.providerConfigs!.map((cfg) => (
                <div className="settings-list-item static" key={cfg.providerKey}>
                  <div>
                    <strong>{cfg.serverName || cfg.providerKey}</strong>
                    <span>
                      {cfg.providerKey}
                      {cfg.configPath ? ` · ${cfg.configPath}` : ""}
                    </span>
                    {cfg.lastError ? <span style={{ color: "var(--err)" }}>{cfg.lastError}</span> : null}
                  </div>
                  <GdriveBadge value={cfg.status} />
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Validation modal */}
      {showValidation && validation ? (
        <div className="settings-modal-backdrop" onClick={() => setShowValidation(false)}>
          <div className="settings-modal" onClick={(e) => e.stopPropagation()}>
            <div className="settings-panel-head">
              <div>
                <div className="settings-eyebrow">Setup check</div>
                <h3 style={{ margin: "4px 0 8px" }}>Validation results</h3>
              </div>
              <GdriveBadge value={validation.valid ? "configured" : "needs_input"} />
            </div>
            <div className="settings-list" style={{ marginTop: 16 }}>
              {validation.checks.map((check) => (
                <div className="settings-list-item static" key={check.key} style={{ flexDirection: "column", alignItems: "flex-start" }}>
                  <strong style={{ color: check.status === "passed" ? "var(--ok)" : check.status === "failed" ? "var(--err)" : "var(--text-dim)" }}>
                    {check.message}
                  </strong>
                  <span>{check.key} — {check.status}</span>
                </div>
              ))}
            </div>
            <div className="settings-actions">
              <button className="secondary-btn" onClick={() => setShowValidation(false)} type="button">Close</button>
            </div>
          </div>
        </div>
      ) : null}
    </section>
  );
}
