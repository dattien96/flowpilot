import React, { useEffect, useMemo, useState } from "react";
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
  accountHomePath: string;
  configPath: string;
  status: "not_started" | "configured" | "config_stale" | "failed";
  configKind?: "proxy" | "legacy_raw" | "unknown";
  command?: string;
  args?: string[];
  mode?: "read_only" | "read_write" | "";
  approvalMode?: string;
  lastCheckedAt?: string;
  lastError?: string;
  serverName?: string;
  changed?: boolean;
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

const GOOGLE_DRIVE_CALLBACK_PATH = "/artifact-storage/google-drive/oauth/callback";
const DEFAULT_REDIRECT_URI = `${RUNNER_URL}${GOOGLE_DRIVE_CALLBACK_PATH}`;
const PICKER_ALLOWED_REFERRERS = pickerAllowedReferrers(RUNNER_URL);

function pickerAllowedReferrers(runnerUrl: string): string[] {
  const url = new URL(runnerUrl);
  const port = url.port ? `:${url.port}` : "";
  const hosts = new Set([url.hostname]);
  if (url.hostname === "127.0.0.1") hosts.add("localhost");
  if (url.hostname === "localhost") hosts.add("127.0.0.1");
  return Array.from(hosts).map((host) => `${url.protocol}//${host}${port}/*`);
}
const PROVIDER_LABELS: Record<string, string> = { codex: "Codex", gemini: "Gemini", claude: "Claude", grok: "Grok" };

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
  return sync.clientId?.trim() && sync.redirectUri?.trim() && sync.hasClientSecret ? "configured" : "needs_input";
}

function providerSetupStatus(status: GoogleDriveWorkspaceConfigResponse | null) {
  const configs = status?.providerConfigs ?? [];
  if (configs.length === 0) return "not_started";
  if (configs.some((c) => c.status === "failed")) return "failed";
  if (configs.every((c) => c.status === "configured")) return "configured";
  if (configs.some((c) => c.status === "config_stale")) return "config_stale";
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
  if (value === "configured" || value === "online" || value === "connected" || value === "ready" ||
      value === "read ready" || value === "proxy read ready" || value === "artifact write ready") cls += " gdrive-badge-ok";
  else if (value === "needs_input" || value === "needs_auth" || value === "reconnect_required" ||
           value === "warning" || value === "config_stale" || value === "proxy read missing scope" ||
           value === "artifact write missing scope") cls += " gdrive-badge-warn";
  else if (value === "failed") cls += " gdrive-badge-err";
  return <span className={cls}>{value}</span>;
}

function CopyBtn({ value, small }: { value: string; small?: boolean }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      className={`ghost-btn gdrive-copy-btn${small ? " small" : ""}`}
      type="button"
      onClick={() => {
        void navigator.clipboard.writeText(value).then(() => {
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        });
      }}
    >
      {copied ? "✓" : "⎘"}
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

function StepConnector({ done }: { done: boolean }) {
  return (
    <div className="gdrive-connector-wrap">
      <div className={`gdrive-connector-line${done ? " done" : ""}`} />
    </div>
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
  const [configureMessage, setConfigureMessage] = useState<string | null>(null);
  const [configureError, setConfigureError] = useState<string | null>(null);

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
        setArtifactForm((prev) => ({
          clientId: payload.artifactSync.clientId ?? "",
          clientSecret: "",
          redirectUri: payload.artifactSync.redirectUri ?? prev.redirectUri,
        }));
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

  useEffect(() => { void loadStatus(); }, []);
  useEffect(() => { setExpandedAccounts({}); }, [activeStep]);
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

  const providerConfigs = status?.providerConfigs ?? [];
  const actionableProviderConfigs = providerConfigs.filter((c) => c.accountHomePath?.trim().length > 0);
  const pendingConfigs = providerConfigs.filter((c) => c.status !== "configured");
  const proxyMcpConfigured = Boolean(status?.mcp.configured);
  const canConfigureProviders = proxyMcpConfigured && actionableProviderConfigs.length > 0;

  const saveArtifactSync = async () => {
    setBusyAction("save-artifact"); setMessage(null);
    try {
      const res = await runnerFetch("/google-drive-config", {
        method: "PUT", headers: { "content-type": "application/json" },
        body: JSON.stringify({ clientId: artifactForm.clientId, clientSecret: artifactForm.clientSecret, redirectUri: artifactForm.redirectUri }),
      });
      if (!res.ok) throw new Error(await readError(res));
      await loadStatus();
      setArtifactForm((prev) => ({ ...prev, clientSecret: "" }));
      setMessage("Artifact sync configuration saved.");
    } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to save."); }
    finally { setBusyAction(null); }
  };

  const savePickerApiKey = async () => {
    setBusyAction("save-picker"); setMessage(null);
    try {
      const res = await runnerFetch("/google-drive-config", {
        method: "PUT", headers: { "content-type": "application/json" },
        body: JSON.stringify({ pickerApiKey }),
      });
      if (!res.ok) throw new Error(await readError(res));
      await loadStatus(); setPickerApiKey(""); setMessage("Google Picker API key saved.");
    } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to save."); }
    finally { setBusyAction(null); }
  };

  const saveProxyAccount = async () => {
    setBusyAction("save-proxy-account"); setMessage(null);
    try {
      const res = await runnerFetch("/google-drive-config", {
        method: "PUT", headers: { "content-type": "application/json" },
        body: JSON.stringify({ mcpAccountId: mcpAccountId.trim() }),
      });
      if (!res.ok) throw new Error(await readError(res));
      await loadStatus(); setMessage("Google account selection saved.");
    } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to save."); }
    finally { setBusyAction(null); }
  };

  const startAccountConnect = async (accountId?: string) => {
    setBusyAction(accountId ? `reconnect:${accountId}` : "connect-account"); setMessage(null);
    try {
      const res = await runnerFetch("/google-drive-config/accounts/connect-sessions", {
        method: "POST", headers: { "content-type": "application/json" },
        body: JSON.stringify(accountId ? { accountId } : {}),
      });
      const payload = await res.json().catch(() => ({})) as { connectUrl?: string; error?: string };
      if (!res.ok) throw new Error(payload.error ?? "Unable to start connection.");
      if (typeof payload.connectUrl === "string" && payload.connectUrl.trim()) {
        window.open(payload.connectUrl, "_blank", "width=980,height=820");
      }
      setMessage(accountId ? "Reconnect started — complete in the popup." : "Connect started — complete in the popup.");
    } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to start account connection."); }
    finally { setBusyAction(null); }
  };

  const disconnectAccount = async (accountId: string) => {
    setBusyAction(`disconnect:${accountId}`); setMessage(null);
    try {
      const res = await runnerFetch(`/google-drive-config/accounts/${encodeURIComponent(accountId)}`, { method: "DELETE" });
      if (!res.ok && res.status !== 204) throw new Error(await readError(res));
      await loadStatus();
      if (mcpAccountId === accountId) setMcpAccountId("");
      setMessage("Google Drive account disconnected.");
    } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to disconnect."); }
    finally { setBusyAction(null); }
  };

  const configureAiProviders = async () => {
    setBusyAction("configure-providers");
    setConfigureMessage(null); setConfigureError(null);
    const errors: string[] = [];
    for (const cfg of actionableProviderConfigs) {
      try {
        const res = await runnerFetch("/google-drive-config/mcp-provider-config/ensure", {
          method: "POST", headers: { "content-type": "application/json" },
          body: JSON.stringify({ providerKey: cfg.providerKey, accountHomePath: cfg.accountHomePath, scope: "account", mode: "read_only" }),
        });
        if (!res.ok) throw new Error(await readError(res));
      } catch (error) {
        errors.push(`${PROVIDER_LABELS[cfg.providerKey] ?? cfg.providerKey}: ${error instanceof Error ? error.message : "Unknown error"}`);
      }
    }
    await loadStatus();
    if (errors.length > 0) {
      setConfigureError(errors.join("\n"));
    } else {
      const count = actionableProviderConfigs.length;
      setConfigureMessage(
        pendingConfigs.length === 0
          ? `Refreshed ${count} provider${count === 1 ? "" : "s"} successfully.`
          : `Applied configuration to ${count} provider${count === 1 ? "" : "s"} successfully.`,
      );
    }
    setBusyAction(null);
  };

  const validateSetup = async () => {
    setBusyAction("validate"); setMessage(null);
    try {
      const payload: Record<string, string> = { clientId: artifactForm.clientId, clientSecret: artifactForm.clientSecret, redirectUri: artifactForm.redirectUri, pickerApiKey };
      if (mcpAccountId.trim()) payload.mcpAccountId = mcpAccountId.trim();
      const res = await runnerFetch("/google-drive-config/validate", {
        method: "POST", headers: { "content-type": "application/json" },
        body: JSON.stringify(payload),
      });
      const result = await res.json() as GoogleDriveValidationResult;
      setValidation(result); setShowValidation(true);
      setMessage(result.valid ? "Google Drive checks passed." : "Fix the failed checks before saving.");
    } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to validate."); }
    finally { setBusyAction(null); }
  };

  const resetConfig = async () => {
    setBusyAction("reset"); setMessage(null);
    try {
      const res = await runnerFetch("/google-drive-config", { method: "DELETE" });
      if (!res.ok) throw new Error(await readError(res));
      localStorage.removeItem("flowpilot_gdrive_step1_done");
      localStorage.removeItem("flowpilot_gdrive_step2_done");
      localStorage.removeItem("flowpilot_gdrive_step3_done");
      setStep1Done(false); setStep2Done(false); setStep3Done(false); setValidation(null);
      await loadStatus(); setMessage("Google Drive config reset.");
    } catch (error) { setMessage(error instanceof Error ? error.message : "Unable to reset."); }
    finally { setBusyAction(null); }
  };

  const confirmStep = (n: 1 | 2 | 3) => {
    const key = `flowpilot_gdrive_step${n}_done`;
    localStorage.setItem(key, "true");
    if (n === 1) { setStep1Done(true); setActiveStep(2); }
    else if (n === 2) { setStep2Done(true); setActiveStep(3); }
    else { setStep3Done(true); setActiveStep(4); }
  };

  const steps = [
    { index: 1, title: "Step 1", subtitle: "Create project", status: stepSummary.project },
    { index: 2, title: "Step 2", subtitle: "Configure OAuth consent screen", status: stepSummary.consent },
    { index: 3, title: "Step 3", subtitle: "Enable APIs", status: stepSummary.apis },
    { index: 4, title: "Step 4", subtitle: "Create Web OAuth client", status: artifactSyncStepStatus(status?.artifactSync) },
    { index: 5, title: "Step 5", subtitle: "Create API key", status: status?.artifactSync.hasPickerApiKey ? "configured" : "needs_input" },
    { index: 6, title: "Step 6", subtitle: "Select Google account", status: status?.mcp.status ?? "not_started" },
    { index: 7, title: "Step 7", subtitle: "Proxy MCP setup", status: providerSetupStatus(status) },
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
        <div className="settings-feedback" style={{ marginTop: 12 }}>
          {message}
          {validation && !validation.valid ? (
            <button className="ghost-btn" style={{ marginLeft: 8 }} onClick={() => setShowValidation(true)} type="button">View checks</button>
          ) : null}
        </div>
      ) : null}
      {status?.lastError ? (
        <div className="settings-feedback error" style={{ marginTop: 8 }}>{status.lastError}</div>
      ) : null}

      {/* Stepper */}
      <div className="settings-subpanel">
        <div className="gdrive-stepper">
          {steps.map((step, idx) => {
            const done = isStepConfigured(step.status);
            const active = activeStep === step.index;
            const isErr = step.status === "failed";
            const isWarn = step.status === "needs_input" || step.status === "reconnect_required" || step.status === "needs_auth" || step.status === "config_stale" || step.status === "warning";
            const prevDone = idx > 0 ? isStepConfigured(steps[idx - 1].status) : false;

            return (
              <React.Fragment key={step.index}>
                {idx > 0 && <StepConnector done={prevDone} />}
                <button
                  className={`gdrive-step${active ? " active" : ""}${done ? " done" : ""}${isErr ? " err" : ""}${isWarn && !done ? " warn" : ""}`}
                  onClick={() => setActiveStep(step.index)}
                  type="button"
                >
                  <span className="gdrive-step-eyebrow">{step.title}</span>
                  <div className={`gdrive-step-circle${done ? " done" : ""}${isErr ? " err" : ""}${isWarn && !done ? " warn" : ""}${active && !done ? " active" : ""}`}>
                    {done ? "✓" : isErr ? "✕" : isWarn ? "△" : step.index}
                  </div>
                  <span className={`gdrive-step-label${active ? " active" : ""}`}>{step.subtitle}</span>
                </button>
              </React.Fragment>
            );
          })}
        </div>
      </div>

      {/* Step 1 */}
      {activeStep === 1 && (
        <div className="settings-subpanel">
          <div className="gdrive-step-header">
            <div>
              <div className="settings-eyebrow">Step 1</div>
              <h3 style={{ marginTop: 4 }}>Create project</h3>
              <p className="gdrive-step-copy">Use one Google Cloud project for this MVP and keep the active project selected.</p>
            </div>
            <GdriveBadge value={stepSummary.project} />
          </div>
          <GuideSteps items={[
            "Open Google Cloud Console.",
            "Use the top project selector.",
            "Create one project such as FlowPilot Local if needed.",
            "Make sure that project is the active one before creating clients or keys.",
          ]} />
          <div className="settings-actions">
            <button className="primary-btn" onClick={() => confirmStep(1)} type="button">Confirm it done</button>
            <button className="secondary-btn" onClick={() => setActiveStep(2)} type="button">Next step</button>
          </div>
        </div>
      )}

      {/* Step 2 */}
      {activeStep === 2 && (
        <div className="settings-subpanel">
          <div className="gdrive-step-header">
            <div>
              <div className="settings-eyebrow">Step 2</div>
              <h3 style={{ marginTop: 4 }}>Configure OAuth consent screen</h3>
              <p className="gdrive-step-copy">Use External for personal testing, add test users, switch to Production when done testing.</p>
            </div>
            <GdriveBadge value={stepSummary.consent} />
          </div>
          <GuideSteps
            items={[
              "Open Google Auth Platform.",
              "Open Audience.",
              "Choose External for personal MVP and friend testing.",
              "Add every Google account you will use as a test user if the app is still in Testing mode.",
              "Move to Production when you want to avoid short-lived testing refresh tokens.",
            ]}
            links={[{ label: "Open Google OAuth consent guide", href: "https://developers.google.com/workspace/guides/configure-oauth-consent" }]}
          />
          <div className="settings-actions">
            <button className="primary-btn" onClick={() => confirmStep(2)} type="button">Confirm it done</button>
            <button className="secondary-btn" onClick={() => setActiveStep(3)} type="button">Next step</button>
          </div>
        </div>
      )}

      {/* Step 3 */}
      {activeStep === 3 && (
        <div className="settings-subpanel">
          <div className="gdrive-step-header">
            <div>
              <div className="settings-eyebrow">Step 3</div>
              <h3 style={{ marginTop: 4 }}>Enable APIs</h3>
              <p className="gdrive-step-copy">Enable Google Drive API, Google Picker API, and the Docs/Sheets/Slides APIs if the MCP tools need them.</p>
            </div>
            <GdriveBadge value={stepSummary.apis} />
          </div>
          <GuideSteps items={[
            "Open APIs & Services > Library.",
            "Enable Google Drive API.",
            "Enable Google Picker API.",
            "Enable Cloud Translation API — required for the in-chat Translate to Vietnamese feature (free up to 500k chars/month).",
            "Also enable Google Docs API, Google Sheets API, and Google Slides API if you want broader MCP support.",
          ]} />
          <div className="settings-actions">
            <button className="primary-btn" onClick={() => confirmStep(3)} type="button">Confirm it done</button>
            <button className="secondary-btn" onClick={() => setActiveStep(4)} type="button">Next step</button>
          </div>
        </div>
      )}

      {/* Step 4 */}
      {activeStep === 4 && (
        <div className="settings-subpanel">
          <div className="gdrive-step-header">
            <div>
              <div className="settings-eyebrow">Step 4</div>
              <h3 style={{ marginTop: 4 }}>Create Web OAuth client</h3>
              <p className="gdrive-step-copy">Save the Web OAuth client locally so FlowPilot can reuse it for artifact sync and the proxy MCP without editing .env.</p>
            </div>
            <GdriveBadge value={artifactSyncStepStatus(status?.artifactSync)} />
          </div>
          <GuideSteps
            items={[
              "Open Google Auth Platform > Clients.",
              "Click Create client.",
              "Choose Web application.",
              "Use a clear name such as FlowPilot Google Drive Local.",
              "Add the redirect URI shown below.",
              "Save the client, then copy Client ID and Client Secret into this form.",
            ]}
            links={[{ label: "Open Google credential creation guide", href: "https://developers.google.com/workspace/guides/create-credentials" }]}
          />
          <CodeRow label="Required redirect URI" value={DEFAULT_REDIRECT_URI} />
          <div className="settings-grid" style={{ marginTop: 14 }}>
            <label className="settings-field">
              <span>Client ID</span>
              <input placeholder="Insert Client ID here" value={artifactForm.clientId} autoComplete="off"
                onChange={(e) => setArtifactForm((p) => ({ ...p, clientId: e.target.value }))} />
            </label>
            <label className="settings-field">
              <span>Client Secret <span style={{ color: "var(--text-dim)", fontWeight: 400 }}>(leave blank to keep stored)</span></span>
              <div className="gdrive-secret-row">
                <input placeholder="Google OAuth client secret" type={showSecret ? "text" : "password"}
                  value={artifactForm.clientSecret} autoComplete="new-password" style={{ flex: 1 }}
                  onChange={(e) => setArtifactForm((p) => ({ ...p, clientSecret: e.target.value }))} />
                <button className="ghost-btn" onClick={() => setShowSecret((v) => !v)} type="button" style={{ padding: "0 10px" }}>
                  {showSecret ? "Hide" : "Show"}
                </button>
              </div>
            </label>
            <label className="settings-field settings-field-full">
              <span>Redirect URI</span>
              <div className="gdrive-secret-row">
                <input value={artifactForm.redirectUri} style={{ flex: 1 }}
                  onChange={(e) => setArtifactForm((p) => ({ ...p, redirectUri: e.target.value }))} />
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

      {/* Step 5 */}
      {activeStep === 5 && (
        <div className="settings-subpanel">
          <div className="gdrive-step-header">
            <div>
              <div className="settings-eyebrow">Step 5</div>
              <h3 style={{ marginTop: 4 }}>Create API key for Google Picker</h3>
              <p className="gdrive-step-copy">Save the browser-side Picker API key separately from the OAuth client secret.</p>
            </div>
            <GdriveBadge value={status?.artifactSync.hasPickerApiKey ? "configured" : "needs_input"} />
          </div>
          <GuideSteps items={[
            "Open APIs & Services > Credentials.",
            "Click Create Credentials > API key.",
            "Give it a clear name such as FlowPilot Picker API Key.",
            "Restrict the key to Google Picker API.",
            "For local restricted keys, set Application restrictions to Websites and add the HTTP referrers below.",
            "For the simplest local MVP test only, keep Application restriction as None, then paste the key here.",
          ]} />
          <div className="gdrive-referrer-grid">
            {PICKER_ALLOWED_REFERRERS.map((ref) => (
              <CodeRow key={ref} label="Allowed picker referrer" value={ref} />
            ))}
          </div>
          <div className="settings-grid" style={{ marginTop: 14 }}>
            <label className="settings-field settings-field-full">
              <span>Picker API key <span style={{ color: "var(--text-dim)", fontWeight: 400 }}>(leave blank to keep stored)</span></span>
              <div className="gdrive-secret-row">
                <input placeholder="Google Picker API key" type={showPickerKey ? "text" : "password"}
                  value={pickerApiKey} autoComplete="new-password" style={{ flex: 1 }}
                  onChange={(e) => setPickerApiKey(e.target.value)} />
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

      {/* Step 6 */}
      {activeStep === 6 && (
        <div className="settings-subpanel">
          <div className="gdrive-step-header">
            <div>
              <div className="settings-eyebrow">Step 6</div>
              <h3 style={{ marginTop: 4 }}>Select Google account for proxy MCP</h3>
              <p className="gdrive-step-copy">Choose one connected Google account for the proxy MCP. Project artifact storage keeps its own per-project account and folder binding.</p>
            </div>
            <GdriveBadge value={status?.mcp.status ?? "not_started"} />
          </div>
          <div className="settings-actions" style={{ marginTop: 12 }}>
            <button className="primary-btn" disabled={busyAction !== null} onClick={() => void startAccountConnect()} type="button">
              {busyAction === "connect-account" ? "Opening..." : "Connect Google account"}
            </button>
            <p style={{ fontSize: 12, color: "var(--text-dim)", alignSelf: "center", margin: 0 }}>
              Connect at least one account before selecting the proxy MCP account or binding project folders.
            </p>
          </div>
          <div className="settings-grid" style={{ marginTop: 14 }}>
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
              <span>Select the Google account whose refresh token the proxy MCP should use.</span>
            </label>
          </div>
          {selectedAccount ? (
            <div className="settings-card" style={{ marginTop: 12 }}>
              <div className="settings-card-head">
                <strong>Current account selection</strong>
                <GdriveBadge value={selectedAccount.status} />
              </div>
              <div className="gdrive-badge-row">
                <GdriveBadge value={selectedAccount.mcpReadReady ? "proxy read ready" : "proxy read missing scope"} />
                <GdriveBadge value={selectedAccount.mcpWriteReady ? "artifact write ready" : "artifact write missing scope"} />
              </div>
              {selectedAccount.grantedScopes?.length ? (
                <p style={{ marginTop: 8, fontSize: 12, color: "var(--text-dim)" }}>
                  Granted scopes: {selectedAccount.grantedScopes.join(", ")}
                </p>
              ) : null}
              {selectedAccount.missingScopes?.length ? (
                <p style={{ marginTop: 4, fontSize: 12, color: "var(--warn)" }}>
                  Missing scopes: {selectedAccount.missingScopes.join(", ")}
                </p>
              ) : null}
            </div>
          ) : (
            <div className="settings-card" style={{ marginTop: 12 }}>
              <p style={{ fontSize: 13, color: "var(--text-dim)", margin: 0 }}>
                {status?.mcp.accountSelectionRequired
                  ? "Select a connected account to finish proxy MCP setup."
                  : status?.mcp.accountId
                    ? `Using account ${status.mcp.accountEmail ? `${status.mcp.accountEmail} ` : ""}(${status.mcp.accountId})`
                    : "No account selected yet."}
              </p>
            </div>
          )}
          <div className="settings-actions">
            <button className="primary-btn" disabled={busyAction === "save-proxy-account" || !mcpAccountId.trim()} onClick={() => void saveProxyAccount()} type="button">
              {busyAction === "save-proxy-account" ? "Saving..." : "Save account selection"}
            </button>
          </div>
          {/* Account list */}
          {proxyAccounts.length === 0 ? (
            <div className="settings-empty" style={{ marginTop: 14 }}>
              No Google accounts connected yet. Click Connect Google account above, complete OAuth in the popup, then come back here to select the proxy MCP account.
            </div>
          ) : (
            <div className="settings-list" style={{ marginTop: 14 }}>
              {proxyAccounts.map((acc) => {
                const isExpanded = expandedAccounts[acc.accountId] ?? false;
                const savedSelection = acc.accountId === status?.mcp.accountId;
                return (
                  <div key={acc.accountId} className={`settings-list-item static${savedSelection ? " active" : ""}`} style={{ flexDirection: "column", alignItems: "stretch" }}>
                    <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <strong style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
                          {acc.accountEmail || acc.accountId}
                          {savedSelection ? <span className="gdrive-badge gdrive-badge-ok">saved</span> : null}
                        </strong>
                        <div style={{ marginTop: 4, display: "flex", gap: 6, flexWrap: "wrap" }}>
                          <GdriveBadge value={acc.status} />
                          <GdriveBadge value={acc.mcpReadReady ? "read ready" : "read missing scope"} />
                          <GdriveBadge value={`${acc.projectCount} project${acc.projectCount === 1 ? "" : "s"}`} />
                        </div>
                      </div>
                      <button className="ghost-btn" onClick={() => setExpandedAccounts((p) => ({ ...p, [acc.accountId]: !p[acc.accountId] }))} type="button">
                        {isExpanded ? "▲" : "▼"}
                      </button>
                    </div>
                    {isExpanded && (
                      <div style={{ marginTop: 12, borderTop: "1px solid var(--border)", paddingTop: 12 }}>
                        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 4, fontSize: 12, color: "var(--text-dim)" }}>
                          <p style={{ margin: 0 }}>Granted scopes: {acc.grantedScopes?.join(", ") || "none"}</p>
                          <p style={{ margin: 0 }}>Missing scopes: {acc.missingScopes?.join(", ") || "none"}</p>
                          <p style={{ margin: 0 }}>Connected: {acc.connectedAt || "unknown"}</p>
                          <p style={{ margin: 0 }}>Updated: {acc.updatedAt || "unknown"}</p>
                          {acc.oauthClientId ? <p style={{ margin: 0, gridColumn: "1 / -1" }}>OAuth client: {acc.oauthClientId}</p> : null}
                        </div>
                        {acc.lastError ? <p style={{ fontSize: 12, color: "var(--err)", marginTop: 8, marginBottom: 0 }}>{acc.lastError}</p> : null}
                        <div className="settings-actions" style={{ marginTop: 10 }}>
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
          <div className="settings-actions" style={{ marginTop: 14 }}>
            <button className="secondary-btn" onClick={() => setActiveStep(7)} type="button">Next step</button>
          </div>
        </div>
      )}

      {/* Step 7 */}
      {activeStep === 7 && (
        <div className="settings-subpanel">
          <div className="gdrive-step-header" style={{ marginBottom: 0 }}>
            <div>
              <div className="settings-eyebrow">Step 7</div>
              <h3 style={{ marginTop: 4 }}>Google Drive proxy MCP setup</h3>
              <p className="gdrive-step-copy">Configure Codex, Gemini, Claude, and Grok against the FlowPilot proxy MCP.</p>
            </div>
            <button
              className="primary-btn"
              disabled={!canConfigureProviders || busyAction === "configure-providers"}
              onClick={() => void configureAiProviders()}
              type="button"
            >
              {busyAction === "configure-providers" ? "Configuring..." : "Configure AI Providers"}
            </button>
          </div>

          {/* Proxy MCP readiness guidance */}
          {!proxyMcpConfigured ? (
            <div className="settings-feedback" style={{ marginTop: 14 }}>
              <strong>
                {status?.mcp.accountSelectionRequired ? "Proxy MCP needs an active Google account selection." :
                  status?.mcp.reconnectRequired ? "Proxy MCP needs the selected Google account to be reconnected." :
                  status?.mcp.missingScopes?.length ? "Proxy MCP is missing required Google Drive scopes." :
                  "Proxy MCP is not ready yet."}
              </strong>
              <span style={{ display: "block", marginTop: 4 }}>
                {status?.mcp.accountSelectionRequired ? "Select an active Google account in Step 6 before configuring provider MCPs." :
                  status?.mcp.reconnectRequired ? `Reconnect ${status?.mcp.accountEmail || "the selected account"} in Step 6 before configuring provider MCPs.` :
                  status?.mcp.missingScopes?.length ? `Reconnect and grant these scopes: ${status.mcp.missingScopes.join(", ")}.` :
                  "Complete Google Drive setup so the proxy MCP can supply the provider-side MCP configs."}
              </span>
            </div>
          ) : (
            <div className="settings-feedback" style={{ marginTop: 14, borderColor: "rgba(63, 185, 80, 0.3)", background: "rgba(63, 185, 80, 0.06)", color: "var(--ok)" }}>
              <strong>FlowPilot proxy MCP is active in this runner.</strong>
              <span style={{ display: "block", marginTop: 4, color: "var(--text-dim)" }}>
                The rows below show the detected MCP type and command from each provider config file.
              </span>
            </div>
          )}

          {configureMessage ? (
            <div className="settings-feedback" style={{ marginTop: 8, borderColor: "rgba(63, 185, 80, 0.3)", background: "rgba(63, 185, 80, 0.06)", color: "var(--ok)" }}>
              {configureMessage}
            </div>
          ) : null}
          {configureError ? (
            <div className="settings-feedback error" style={{ marginTop: 8, whiteSpace: "pre-wrap" }}>{configureError}</div>
          ) : null}

          {providerConfigs.length === 0 ? (
            <div className="settings-empty" style={{ marginTop: 14 }}>
              No provider configurations available. Complete steps 4–6 first, then restart the runner to generate provider configs.
            </div>
          ) : (
            <div className="settings-list" style={{ marginTop: 14 }}>
              {providerConfigs.map((cfg) => (
                <div className="settings-list-item static" key={`${cfg.providerKey}:${cfg.accountHomePath || cfg.configPath || "?"}`} style={{ flexDirection: "column", alignItems: "stretch" }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <strong style={{ display: "flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
                        {PROVIDER_LABELS[cfg.providerKey] ?? cfg.providerKey}
                        <GdriveBadge value={cfg.status} />
                      </strong>
                      {cfg.accountHomePath ? <span style={{ display: "block" }}>Home: {cfg.accountHomePath}</span> : null}
                      {cfg.configPath ? <span style={{ display: "block" }}>Config: {cfg.configPath}</span> : null}
                      {cfg.configKind ? <span style={{ display: "block" }}>Type: {cfg.configKind === "proxy" ? "Proxy MCP" : cfg.configKind === "legacy_raw" ? "Legacy Raw MCP" : "Unknown"}</span> : null}
                      {cfg.mode ? <span style={{ display: "block" }}>Access: {cfg.mode === "read_write" ? "Read + write" : cfg.mode === "read_only" ? "Read only" : cfg.mode}</span> : null}
                      {cfg.lastError ? <span style={{ display: "block", color: "var(--err)" }}>Error: {cfg.lastError}</span> : null}
                    </div>
                  </div>
                  {cfg.command ? (
                    <div className="gdrive-code-row" style={{ marginTop: 8 }}>
                      <div className="gdrive-code-label">Detected command</div>
                      <div className="gdrive-code-block">
                        <code className="gdrive-code">{[cfg.command, ...(cfg.args ?? [])].join(" ")}</code>
                      </div>
                    </div>
                  ) : null}
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
                <div className="settings-eyebrow">Step check</div>
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
