import { createFileRoute, Link, useRouter } from "@tanstack/react-router";
import { AlertCircle, Check, ChevronDown, ChevronRight, Copy, Eye, EyeOff, TriangleAlert } from "lucide-react";
import { useEffect, useMemo, useState, type ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { PageFrame } from "@/components/common/page-frame";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { loadMcpSettingsData } from "@/features/mcp/mcp-settings-loader";
import {
  loadGoogleDriveRuntimeStatus,
  type GoogleDriveRuntimeStatus,
  type GoogleDriveValidationResult,
} from "@/lib/google-drive/runtime-config";
import { Badge } from "@/presentation/components/ui/badge";
import { GoogleDriveProviderConfigCard } from "./components/-GoogleDriveProviderConfigCard";

const DEFAULT_REDIRECT_URI =
  "http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback";
const PICKER_ALLOWED_REFERRERS = [
  "http://127.0.0.1:4317/*",
  "http://localhost:4317/*",
];

function isStepConfigured(value: string) {
  return value === "configured" || value === "online" || value === "connected" || value === "ready";
}

export const Route = createFileRoute("/_authenticated/settings/google-drive-setup")({
  loader: loadMcpSettingsData,
  component: GoogleDriveSetupPage,
});

export function GoogleDriveSetupPage() {
  Route.useLoaderData();
  const router = useRouter();
  const [status, setStatus] = useState<GoogleDriveRuntimeStatus | null>(null);
  const [clientIdEditable, setClientIdEditable] = useState(false);
  const [artifactForm, setArtifactForm] = useState({
    clientId: "",
    clientSecret: "",
    redirectUri: DEFAULT_REDIRECT_URI,
  });
  const [pickerApiKey, setPickerApiKey] = useState("");
  const [mcpAccountId, setMcpAccountId] = useState("");
  const [showClientSecret, setShowClientSecret] = useState(false);
  const [showPickerApiKey, setShowPickerApiKey] = useState(false);
  const [validation, setValidation] = useState<GoogleDriveValidationResult | null>(null);
  const [validationOpen, setValidationOpen] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [busyAction, setBusyAction] = useState<string | null>(null);
  const [activeStep, setActiveStep] = useState<number>(1);
  const [expandedAccounts, setExpandedAccounts] = useState<Record<string, boolean>>({});

  const [step1Done, setStep1Done] = useState(() => {
    if (typeof window !== "undefined") {
      return localStorage.getItem("flowpilot_gdrive_step1_done") === "true";
    }
    return false;
  });
  const [step2Done, setStep2Done] = useState(() => {
    if (typeof window !== "undefined") {
      return localStorage.getItem("flowpilot_gdrive_step2_done") === "true";
    }
    return false;
  });
  const [step3Done, setStep3Done] = useState(() => {
    if (typeof window !== "undefined") {
      return localStorage.getItem("flowpilot_gdrive_step3_done") === "true";
    }
    return false;
  });

  const applyLoadedStatus = (nextStatus: GoogleDriveRuntimeStatus) => {
    setStatus(nextStatus);
    setMcpAccountId(
      nextStatus.mcp.accountId?.trim() ||
        (nextStatus.accounts?.length === 1 ? nextStatus.accounts[0].accountId?.trim() : "") ||
        "",
    );
  };

  useEffect(() => {
    let active = true;
    void loadGoogleDriveRuntimeStatus()
      .then((nextStatus) => {
        if (!active) {
          return;
        }
        applyLoadedStatus(nextStatus);
        setArtifactForm({
          clientId: nextStatus.artifactSync.clientId ?? "",
          clientSecret: "",
          redirectUri: nextStatus.artifactSync.redirectUri ?? DEFAULT_REDIRECT_URI,
        });

        // Determine the first step that is not configured
        const initialStep1Done = localStorage.getItem("flowpilot_gdrive_step1_done") === "true";
        const initialStep2Done = localStorage.getItem("flowpilot_gdrive_step2_done") === "true";
        const initialStep3Done = localStorage.getItem("flowpilot_gdrive_step3_done") === "true";
        const stepSum = buildStepSummary(nextStatus, initialStep1Done, initialStep2Done, initialStep3Done);
        const projectConf = isStepConfigured(stepSum.project);
        const consentConf = isStepConfigured(stepSum.consent);
        const apisConf = isStepConfigured(stepSum.apis);
        const s4Conf = isStepConfigured(
          nextStatus.artifactSync.clientId?.trim() &&
            nextStatus.artifactSync.redirectUri?.trim() &&
            nextStatus.artifactSync.hasClientSecret
            ? "configured"
            : "needs_input",
        );
        const s5Conf = isStepConfigured(
          nextStatus.artifactSync.hasPickerApiKey ? "configured" : "needs_input",
        );
        const s6Conf = isStepConfigured(nextStatus.mcp.status ?? "not_started");
        const s7Conf = isStepConfigured(providerSetupStepStatus(nextStatus));

        if (!projectConf) setActiveStep(1);
        else if (!consentConf) setActiveStep(2);
        else if (!apisConf) setActiveStep(3);
        else if (!s4Conf) setActiveStep(4);
        else if (!s5Conf) setActiveStep(5);
        else if (!s6Conf) setActiveStep(6);
        else if (!s7Conf) setActiveStep(7);
        else setActiveStep(1);
      })
      .catch((error) => {
        if (!active) {
          return;
        }
        setMessage(error instanceof Error ? error.message : "Unable to load Google Drive status.");
      });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    setExpandedAccounts({});
  }, [activeStep]);

  useEffect(() => {
    function handleMessage(event: MessageEvent) {
      if (!event.data || typeof event.data !== "object") {
        return;
      }
      if (event.data.type !== "flowpilot-google-drive-account-connected") {
        return;
      }
      void router.invalidate().then(refreshStatus).then(() => {
        setMessage("Google Drive account status refreshed.");
      }).catch((error) => {
        setMessage(error instanceof Error ? error.message : "Unable to refresh Google Drive account status.");
      });
    }

    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [router]);

  const stepSummary = useMemo(
    () => buildStepSummary(status, step1Done, step2Done, step3Done),
    [status, step1Done, step2Done, step3Done],
  );
  const proxyMcpEnabled = true;
  const proxyAccounts = status?.accounts ?? [];
  const selectedProxyAccount =
    proxyAccounts.find((account) => account.accountId === mcpAccountId) ?? null;
  const hasSelectedProxyAccount = mcpAccountId.trim().length > 0;
  const canSaveArtifactSync = artifactForm.clientId.trim().length > 0;
  const canSavePickerApiKey = pickerApiKey.trim().length > 0;
  const canSaveProxyAccount = proxyMcpEnabled && hasSelectedProxyAccount;
  const providerSetupStatus = providerSetupStepStatus(status);

  const refreshStatus = async () => {
    const nextStatus = await loadGoogleDriveRuntimeStatus();
    applyLoadedStatus(nextStatus);
    return nextStatus;
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
      if (mcpAccountId.trim()) {
        payload.mcpAccountId = mcpAccountId.trim();
      }
      const result = (await submitGoogleDriveConfig(
        "/api/runtime/google-drive-config/validate",
        payload,
        "POST",
      )) as GoogleDriveValidationResult;
      setValidation(result);
      setValidationOpen(true);
      setMessage(result.valid ? "Google Drive checks passed." : "Fix the failed checks before saving.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to validate Google Drive config.");
    } finally {
      setBusyAction(null);
    }
  };

  const saveArtifactSync = async () => {
    setBusyAction("save-artifact");
    setMessage(null);
    try {
      const payload: Record<string, string> = {
        clientId: artifactForm.clientId,
        clientSecret: artifactForm.clientSecret,
        redirectUri: artifactForm.redirectUri,
      };
      if (mcpAccountId.trim()) {
        payload.mcpAccountId = mcpAccountId.trim();
      }
      await submitGoogleDriveConfig(
        "/api/runtime/google-drive-config",
        payload,
        "PUT",
      );
      const nextStatus = await refreshStatus();
      setArtifactForm((current) => ({
        clientId: nextStatus.artifactSync.clientId ?? current.clientId,
        redirectUri: nextStatus.artifactSync.redirectUri ?? current.redirectUri,
        clientSecret: "",
      }));
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
      await submitGoogleDriveConfig(
        "/api/runtime/google-drive-config",
        {
          pickerApiKey,
        },
        "PUT",
      );
      await refreshStatus();
      setPickerApiKey("");
      setMessage("Google Picker API key saved.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to save Picker API key.");
    } finally {
      setBusyAction(null);
    }
  };

  const saveProxyAccountSelection = async () => {
    setBusyAction("save-proxy-account");
    setMessage(null);
    try {
      await submitGoogleDriveConfig("/api/runtime/google-drive-config", {
        mcpAccountId: mcpAccountId.trim(),
      }, "PUT");
      const nextStatus = await refreshStatus();
      setMessage(
        nextStatus.mcp.accountSelectionRequired
          ? "Google account selection saved, but the proxy MCP still needs a connected account."
          : "Google account selection saved.",
      );
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to save Google account selection.");
    } finally {
      setBusyAction(null);
    }
  };

  const startAccountConnect = async (accountId?: string) => {
    setBusyAction(accountId ? `reconnect-account:${accountId}` : "connect-account");
    setMessage(null);
    try {
      const response = await fetch("/api/runtime/google-drive-config/accounts/connect-session", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(accountId ? { accountId } : {}),
      });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(payload.error ?? "Unable to start Google Drive account connection.");
      }
      if (typeof payload.connectUrl === "string" && payload.connectUrl.trim()) {
        window.open(payload.connectUrl, "_blank", "width=980,height=820");
      }
      setMessage(accountId ? "Google Drive account reconnect started." : "Google Drive account connect started.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to start Google Drive account connection.");
    } finally {
      setBusyAction(null);
    }
  };

  const disconnectAccount = async (accountId: string) => {
    setBusyAction(`disconnect-account:${accountId}`);
    setMessage(null);
    try {
      const response = await fetch(`/api/runtime/google-drive-config/accounts/${encodeURIComponent(accountId)}`, {
        method: "DELETE",
      });
      if (!response.ok) {
        const payload = await response.json().catch(() => ({}));
        throw new Error(payload.error ?? "Unable to disconnect Google Drive account.");
      }
      const nextStatus = await refreshStatus();
      if (nextStatus.mcp.accountId === accountId) {
        setMcpAccountId("");
      }
      setMessage("Google Drive account disconnected.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to disconnect Google Drive account.");
    } finally {
      setBusyAction(null);
    }
  };

  const handleConfirmStep1 = () => {
    setStep1Done(true);
    if (typeof window !== "undefined") {
      localStorage.setItem("flowpilot_gdrive_step1_done", "true");
    }
    setActiveStep(2);
  };

  const handleConfirmStep2 = () => {
    setStep2Done(true);
    if (typeof window !== "undefined") {
      localStorage.setItem("flowpilot_gdrive_step2_done", "true");
    }
    setActiveStep(3);
  };

  const handleConfirmStep3 = () => {
    setStep3Done(true);
    if (typeof window !== "undefined") {
      localStorage.setItem("flowpilot_gdrive_step3_done", "true");
    }
    setActiveStep(4);
  };

  const resetConfig = async () => {
    setBusyAction("reset");
    setMessage(null);
    try {
      const response = await fetch("/api/runtime/google-drive-config", { method: "DELETE" });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(payload.error ?? "Unable to reset Google Drive config.");
      }
      if (typeof window !== "undefined") {
        localStorage.removeItem("flowpilot_gdrive_step1_done");
        localStorage.removeItem("flowpilot_gdrive_step2_done");
        localStorage.removeItem("flowpilot_gdrive_step3_done");
      }
      setStep1Done(false);
      setStep2Done(false);
      setStep3Done(false);
      const nextStatus = await refreshStatus();
      setValidation(null);
      setMessage(`Google Drive config reset. Current mode: ${nextStatus.artifactSync.source}.`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to reset Google Drive config.");
    } finally {
      setBusyAction(null);
    }
  };

  const steps = [
    {
      index: 1,
      title: "Step 1",
      subtitle: "Create project",
      status: stepSummary.project,
    },
    {
      index: 2,
      title: "Step 2",
      subtitle: "Configure OAuth consent screen",
      status: stepSummary.consent,
    },
    {
      index: 3,
      title: "Step 3",
      subtitle: "Enable APIs",
      status: stepSummary.apis,
    },
    {
      index: 4,
      title: "Step 4",
      subtitle: "Create Web OAuth client",
      status: artifactSyncStepStatus(status?.artifactSync),
    },
    {
      index: 5,
      title: "Step 5",
      subtitle: "Create API key",
      status: status?.artifactSync.hasPickerApiKey ? "configured" : "needs_input",
    },
    {
      index: 6,
      title: "Step 6",
      subtitle: "Select Google account",
      status: status?.mcp.status ?? "not_started",
    },
    {
      index: 7,
      title: "Step 7",
      subtitle: "Proxy MCP setup",
      status: providerSetupStatus,
    },
  ];

  return (
    <PageFrame
      title="Google Console Setup"
      description="Save the Google Cloud values once, then let FlowPilot reuse them for artifact sync and the proxy MCP."
    >
      <section className="rounded-[1.6rem] border border-border bg-card/80 p-6">
        <div className="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex-1 min-w-0">
            <StatusStack status={status} />
            <div className="mt-4 rounded-2xl border border-border/70 bg-background/80 p-4 text-sm text-muted-foreground">
              <p className="font-medium text-foreground">Current paths</p>
              <p className="mt-2 break-all">OAuth callback: {DEFAULT_REDIRECT_URI}</p>
              <p className="mt-1 break-all">
                MCP credentials: {status?.mcp.credentialPath ?? "Not resolved yet"}
              </p>
              <p className="mt-1 break-all">MCP tokens: {status?.mcp.tokenPath ?? "Not resolved yet"}</p>
            </div>
            {message ? (
              <p className="mt-4 rounded-2xl border border-border bg-background px-4 py-3 text-sm">
                {message}
              </p>
            ) : null}
            {status?.lastError ? (
              <p className="mt-3 rounded-2xl border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger">
                {status.lastError}
              </p>
            ) : null}
          </div>

          <div className="flex flex-col gap-3 w-full lg:w-80 shrink-0 border-t border-border/40 pt-4 lg:border-t-0 lg:pt-0">
            <Button
              disabled={busyAction === "reset"}
              onClick={resetConfig}
              variant="secondary"
              className="w-full"
            >
              {busyAction === "reset" ? "Resetting..." : "Reset Google Drive config"}
            </Button>
            
            <Button
              disabled={busyAction === "validate"}
              onClick={validateSetup}
              variant="secondary"
              className="w-full"
            >
              {busyAction === "validate" ? "Validating..." : "Validate setup"}
            </Button>
            
            {validation ? (
              <button
                className="transition hover:opacity-85 mt-1"
                onClick={() => setValidationOpen(true)}
                type="button"
                aria-label="Open last validation result"
              >
                <div className="flex items-center justify-center gap-2">
                  <span className="text-xs uppercase tracking-[0.22em] text-muted-foreground">
                    Latest check:
                  </span>
                  <StatusBadge value={validation.valid ? "configured" : "needs_input"} />
                </div>
              </button>
            ) : null}
          </div>
        </div>
      </section>

      <div className="mt-6 flex flex-col gap-6">
        {/* Stepper Card */}
        <section className="rounded-[1.6rem] border border-border bg-card/80 p-6">
          <div className="w-full overflow-x-auto pb-4 scrollbar-none">
            <div className="min-w-[800px] flex items-start justify-between relative px-6 py-2">
              {steps.map((step, idx) => {
                const stepStatus = step.status;
                const isConfigured = isStepConfigured(stepStatus);
                const isError = stepStatus === "failed";
                const isWarning =
                  stepStatus === "needs_auth" ||
                  stepStatus === "reconnect_required" ||
                  stepStatus === "config_stale" ||
                  stepStatus === "needs_input";
                const isActive = activeStep === step.index;

                return (
                  <div key={step.index} className="flex-1 flex flex-col items-center relative group">
                    {idx < steps.length - 1 && (
                      <div
                        className={`absolute left-[50%] right-[-50%] top-4 h-0.5 z-0 transition-colors duration-300 ${
                          isConfigured ? "bg-success" : "bg-border"
                        }`}
                      />
                    )}

                    <button
                      onClick={() => setActiveStep(step.index)}
                      className="flex flex-col items-center z-10 focus:outline-none w-full"
                      type="button"
                    >
                      <div
                        className={`w-8 h-8 rounded-full flex items-center justify-center transition-all duration-300 ${
                          isConfigured
                            ? "bg-success text-white border-2 border-success shadow-md shadow-success/20"
                            : isError
                              ? "bg-background border-2 border-danger text-danger shadow-md shadow-danger/15"
                              : isWarning
                                ? "bg-background border-2 border-warning text-warning shadow-md shadow-warning/15"
                                : isActive
                                  ? "bg-background border-2 border-success shadow-md shadow-success/15"
                                  : "bg-background border-2 border-border text-muted-foreground"
                        } ${
                          isActive
                            ? isError
                              ? "ring-4 ring-danger/20 scale-105"
                              : isWarning
                                ? "ring-4 ring-warning/20 scale-105"
                                : "ring-4 ring-success/20 scale-105"
                            : "hover:border-muted-foreground/50 hover:scale-105"
                        }`}
                      >
                        {isConfigured ? (
                          <Check className="h-4 w-4 text-white stroke-[3px]" />
                        ) : isError ? (
                          <AlertCircle className="h-4 w-4 text-danger" />
                        ) : isWarning ? (
                          <TriangleAlert className="h-4 w-4 text-warning" />
                        ) : isActive ? (
                          <div className="w-2 h-2 rounded-[0.15rem] bg-success" />
                        ) : (
                          <div className="w-2 h-2 rounded-[0.15rem] bg-muted-foreground/30" />
                        )}
                      </div>

                      <span className={`mt-3 text-xs uppercase tracking-[0.15em] font-semibold transition-colors duration-300 ${isActive ? "text-foreground" : "text-muted-foreground"}`}>
                        {step.title}
                      </span>
                      <span className={`mt-1 text-sm font-medium text-center px-2 transition-colors duration-300 ${isActive ? "text-muted-foreground font-semibold" : "text-muted-foreground/60"}`}>
                        {step.subtitle}
                      </span>
                    </button>
                  </div>
                );
              })}
            </div>
          </div>
        </section>

        {/* Step Content */}
        <div className="transition-all duration-300">
          {activeStep === 1 && (
            <section className="rounded-[1.6rem] border border-border bg-background/80 p-6">
              <SectionHeader
                step="1"
                title="Create project"
                subtitle="Use one Google Cloud project for this MVP and keep the active project selected."
                status={stepSummary.project}
              />
              <div className="mt-4">
                <GuidePanel
                  items={[
                    "Open Google Cloud Console.",
                    "Use the top project selector.",
                    "Create one project such as FlowPilot Local if needed.",
                    "Make sure that project is the active one before creating clients or keys.",
                  ]}
                />
              </div>
              <div className="mt-6 flex flex-wrap gap-3">
                <Button onClick={handleConfirmStep1}>
                  Confirm it done
                </Button>
                <Button onClick={() => setActiveStep(2)} variant="secondary">
                  Next Step
                </Button>
              </div>
            </section>
          )}

          {activeStep === 2 && (
            <section className="rounded-[1.6rem] border border-border bg-background/80 p-6">
              <SectionHeader
                step="2"
                title="Configure OAuth consent screen"
                subtitle="Use External for personal testing, add test users, and switch to Production when you are done testing."
                status={stepSummary.consent}
              />
              <div className="mt-4">
                <GuidePanel
                  items={[
                    "Open Google Auth Platform.",
                    "Open Audience.",
                    "Choose External for personal MVP and friend testing.",
                    "Add every Google account you will use as a test user if the app is still in Testing mode.",
                    "Move to Production when you want to avoid short-lived testing refresh tokens.",
                  ]}
                  links={[
                    {
                      label: "Open Google OAuth consent guide",
                      href: "https://developers.google.com/workspace/guides/configure-oauth-consent",
                    },
                  ]}
                />
              </div>
              <div className="mt-6 flex flex-wrap gap-3">
                <Button onClick={handleConfirmStep2}>
                  Confirm it done
                </Button>
                <Button onClick={() => setActiveStep(3)} variant="secondary">
                  Next Step
                </Button>
              </div>
            </section>
          )}

          {activeStep === 3 && (
            <section className="rounded-[1.6rem] border border-border bg-background/80 p-6">
              <SectionHeader
                step="3"
                title="Enable APIs"
                subtitle="Enable Google Drive API, Google Picker API, and the Docs/Sheets/Slides APIs if the MCP tools need them."
                status={stepSummary.apis}
              />
              <div className="mt-4">
                <GuidePanel
                  items={[
                    "Open APIs & Services > Library.",
                    "Enable Google Drive API.",
                    "Enable Google Picker API.",
                    "Also enable Google Docs API, Google Sheets API, and Google Slides API if you want broader MCP support.",
                  ]}
                />
              </div>
              <div className="mt-6 flex flex-wrap gap-3">
                <Button onClick={handleConfirmStep3}>
                  Confirm it done
                </Button>
                <Button onClick={() => setActiveStep(4)} variant="secondary">
                  Next Step
                </Button>
              </div>
            </section>
          )}

          {activeStep === 4 && (
            <section className="rounded-[1.6rem] border border-border bg-background/80 p-6">
              <SectionHeader
                step="4"
                title="Create Web OAuth client"
                subtitle="Save the Web OAuth client locally so FlowPilot can reuse it for artifact sync and the proxy MCP without editing .env."
                status={artifactSyncStepStatus(status?.artifactSync)}
              />
              <GuidePanel
                className="mt-4"
                items={[
                  "Open Google Auth Platform > Clients.",
                  "Click Create client.",
                  "Choose Web application.",
                  "Use a clear name such as FlowPilot Google Drive Local.",
                  "Add the redirect URI shown below.",
                  "Save the client, then copy Client ID and Client Secret into this form.",
                ]}
                links={[
                  {
                    label: "Open Google credential creation guide",
                    href: "https://developers.google.com/workspace/guides/create-credentials",
                  },
                ]}
              >
                <CodeRow label="Required redirect URI" value={DEFAULT_REDIRECT_URI} />
              </GuidePanel>
              <div aria-hidden="true" className="hidden">
                <input autoComplete="username" tabIndex={-1} type="text" />
                <input autoComplete="current-password" tabIndex={-1} type="password" />
              </div>
              <div className="mt-4 grid gap-4 md:grid-cols-2">
                <Field label="Client ID">
                  <input
                    autoCapitalize="none"
                    autoComplete="new-password"
                    className="w-full rounded-2xl border border-border bg-background px-4 py-3 outline-none transition focus:border-transparent focus:ring-2 focus:ring-accent"
                    data-1p-ignore="true"
                    data-lpignore="true"
                    inputMode="text"
                    name="google-drive-oauth-client-id-value"
                    onChange={(event) => setArtifactForm((current) => ({ ...current, clientId: event.target.value }))}
                    onFocus={() => setClientIdEditable(true)}
                    placeholder="Insert Client ID here"
                    readOnly={!clientIdEditable}
                    spellCheck={false}
                    value={artifactForm.clientId}
                  />
                </Field>
                <SecretField
                  label="Client Secret"
                  note="Leave blank to keep the stored secret."
                  onChange={(value) => setArtifactForm((current) => ({ ...current, clientSecret: value }))}
                  onToggle={() => setShowClientSecret((value) => !value)}
                  placeholder="Google OAuth Web client secret"
                  reveal={showClientSecret}
                  value={artifactForm.clientSecret}
                />
                <Field className="md:col-span-2" label="Redirect URI">
                  <div className="flex overflow-hidden rounded-2xl border border-border bg-background focus-within:ring-2 focus-within:ring-accent">
                    <input
                      className="min-w-0 flex-1 bg-transparent px-4 py-3 outline-none"
                      onChange={(event) =>
                        setArtifactForm((current) => ({ ...current, redirectUri: event.target.value }))
                      }
                      placeholder={DEFAULT_REDIRECT_URI}
                      value={artifactForm.redirectUri}
                    />
                    <CopyButton value={artifactForm.redirectUri} />
                  </div>
                </Field>
              </div>
              <div className="mt-5 flex flex-wrap gap-3">
                <Button disabled={busyAction === "save-artifact" || !canSaveArtifactSync} onClick={saveArtifactSync}>
                  {busyAction === "save-artifact" ? "Saving..." : "Save artifact sync"}
                </Button>
                <Button onClick={() => setActiveStep(5)} variant="secondary">
                  Next Step
                </Button>
              </div>
            </section>
          )}

          {activeStep === 5 && (
            <section className="rounded-[1.6rem] border border-border bg-background/80 p-6">
              <SectionHeader
                step="5"
                title="Create API key for Google Picker"
                subtitle="Save the browser-side Picker API key separately from the OAuth client secret."
                status={status?.artifactSync.hasPickerApiKey ? "configured" : "needs_input"}
              />
              <GuidePanel
                className="mt-4"
                items={[
                  "Open APIs & Services > Credentials.",
                  "Click Create Credentials > API key.",
                  "Give it a clear name such as FlowPilot Picker API Key.",
                  "Restrict the key to Google Picker API.",
                  "Do not enable Authenticate API calls through a service account.",
                  "For local restricted keys, set Application restrictions to Websites and add the full HTTP referrers below. Do not omit http://.",
                  "For the simplest local MVP test only, keep application restriction as None, then copy the key into this field.",
                ]}
              />
              <div className="mt-4 grid gap-3 md:grid-cols-2">
                {PICKER_ALLOWED_REFERRERS.map((referrer) => (
                  <CodeRow key={referrer} label="Allowed picker referrer" value={referrer} />
                ))}
              </div>
              <div className="mt-4 grid gap-4 md:grid-cols-[1fr_auto]">
                <SecretField
                  label="Picker API key"
                  note="Leave blank to keep the stored API key."
                  onChange={setPickerApiKey}
                  onToggle={() => setShowPickerApiKey((value) => !value)}
                  placeholder="Google Picker API key"
                  reveal={showPickerApiKey}
                  value={pickerApiKey}
                />
                <div className="flex items-end gap-3 flex-wrap">
                  <Button
                    disabled={busyAction === "save-picker" || !canSavePickerApiKey}
                    onClick={savePickerApiKey}
                    className="w-full md:w-auto"
                  >
                    {busyAction === "save-picker" ? "Saving..." : "Save picker key"}
                  </Button>
                  <Button onClick={() => setActiveStep(6)} variant="secondary" className="w-full md:w-auto">
                    Next Step
                  </Button>
                </div>
              </div>
            </section>
          )}

          {activeStep === 6 && (
            <section className="rounded-[1.6rem] border border-border bg-background/80 p-6">
              <SectionHeader
                step="6"
                title="Select Google account for proxy MCP"
                subtitle="Choose one connected Google account for the proxy MCP. Project artifact storage keeps its own per-project account and folder binding."
                status={status?.mcp.status ?? "not_started"}
              />
              <div className="mt-4 flex flex-wrap items-center gap-3">
                <Button
                  className="w-full md:w-auto"
                  disabled={busyAction !== null}
                  onClick={() => {
                    void startAccountConnect();
                  }}
                >
                  {busyAction === "connect-account" ? "Opening..." : "Connect Google account"}
                </Button>
                <p className="text-sm text-muted-foreground">
                  Connect at least one Google account here before selecting the proxy MCP account or binding project folders.
                </p>
              </div>
              <div className="mt-4 grid gap-4 lg:grid-cols-[1fr_auto]">
                <Field
                  label={
                    <span className="flex flex-wrap items-center gap-2">
                      Active account
                      <span className="text-xs uppercase tracking-[0.22em] text-muted-foreground">
                        {proxyAccounts.length > 0 ? `${proxyAccounts.length} available` : "none connected"}
                      </span>
                    </span>
                  }
                >
                  <select
                    className="w-full rounded-2xl border border-border bg-background px-4 py-3 outline-none transition focus:border-transparent focus:ring-2 focus:ring-accent"
                    onChange={(event) => setMcpAccountId(event.target.value)}
                    value={mcpAccountId}
                  >
                    <option value="">Select a connected Google account</option>
                    {proxyAccounts.map((account) => {
                      const label = account.accountEmail
                        ? `${account.accountEmail} (${account.accountId})`
                        : account.accountId;
                      const meta = account.projectCount > 0 ? ` - ${account.projectCount} project${account.projectCount === 1 ? "" : "s"}` : "";
                      return (
                        <option key={account.accountId} value={account.accountId}>
                          {label}
                          {meta}
                        </option>
                      );
                    })}
                  </select>
                  <p className="text-xs text-muted-foreground">
                    Select the Google account whose refresh token the proxy MCP should reuse. Project artifact bindings are configured separately per FlowPilot project.
                  </p>
                </Field>
                <div className="flex items-end">
                  <Button
                    disabled={busyAction === "save-proxy-account" || !canSaveProxyAccount}
                    onClick={saveProxyAccountSelection}
                    className="w-full md:w-auto"
                  >
                    {busyAction === "save-proxy-account" ? "Saving..." : "Save account selection"}
                  </Button>
                </div>
              </div>
              <div className="mt-4 rounded-2xl border border-border/70 bg-background/60 p-4 text-sm text-muted-foreground">
                <p className="font-medium text-foreground">Current account selection</p>
                <p className="mt-2 break-words">
                  {status?.mcp.accountSelectionRequired
                    ? "Select a connected account to finish proxy MCP setup."
                    : status?.mcp.accountId
                      ? `Using account ${status.mcp.accountEmail ? `${status.mcp.accountEmail} ` : ""}(${status.mcp.accountId})`
                      : "No account selected yet."}
                </p>
                {selectedProxyAccount ? (
                  <>
                    <div className="mt-3 flex flex-wrap gap-2">
                      <Badge tone={googleDriveAccountBadgeTone(selectedProxyAccount.status)}>
                        {selectedProxyAccount.status}
                      </Badge>
                      <Badge tone={selectedProxyAccount.mcpReadReady ? "success" : "warning"}>
                        {selectedProxyAccount.mcpReadReady ? "proxy read ready" : "proxy read scope missing"}
                      </Badge>
                      <Badge tone={selectedProxyAccount.mcpWriteReady ? "success" : "warning"}>
                        {selectedProxyAccount.mcpWriteReady ? "artifact write ready" : "artifact write scope missing"}
                      </Badge>
                    </div>
                    <p className="mt-2 text-xs">
                      Granted scopes: {formatGoogleDriveScopeSummary(selectedProxyAccount.grantedScopes)}
                    </p>
                    {selectedProxyAccount.missingScopes?.length ? (
                      <p className="mt-1 text-xs text-warning">
                        Missing scopes: {formatGoogleDriveScopeSummary(selectedProxyAccount.missingScopes)}
                      </p>
                    ) : null}
                  </>
                ) : null}
              </div>
              <div className="mt-4 grid gap-4">
                {proxyAccounts.length === 0 ? (
                  <div className="rounded-2xl border border-dashed border-border/70 bg-background/50 p-4 text-sm text-muted-foreground">
                    No Google accounts are connected on this runner yet. Start a new account connection, finish OAuth in the popup, then come back here to select the proxy MCP account or bind project artifact folders.
                  </div>
                ) : (
                  proxyAccounts.map((account) => {
                    const reconnectAction = `reconnect-account:${account.accountId}`;
                    const disconnectAction = `disconnect-account:${account.accountId}`;
                    const selectedInForm = account.accountId === mcpAccountId;
                    const savedSelection = account.accountId === status?.mcp.accountId;
                    const isExpanded = !!expandedAccounts[account.accountId];
                    const toggleExpanded = () => {
                      setExpandedAccounts((prev) => ({
                        ...prev,
                        [account.accountId]: !prev[account.accountId],
                      }));
                    };

                    return (
                      <div
                        key={account.accountId}
                        className={`rounded-2xl border p-4 transition-all duration-300 ${
                          savedSelection
                            ? "border-success/30 bg-success/5"
                            : selectedInForm
                              ? "border-accent/30 bg-accent/5"
                              : "border-border/70 bg-background/50"
                        }`}
                      >
                        <div
                          className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between cursor-pointer select-none hover:opacity-90 transition-opacity duration-200"
                          onClick={toggleExpanded}
                        >
                          <div className="flex items-start gap-3">
                            <div className="mt-1 shrink-0">
                              {isExpanded ? (
                                <ChevronDown className="h-4 w-4 text-muted-foreground" />
                              ) : (
                                <ChevronRight className="h-4 w-4 text-muted-foreground" />
                              )}
                            </div>
                            <div>
                              <div className="flex flex-wrap items-center gap-2">
                                <p className="font-medium text-foreground">
                                  {account.accountEmail || account.accountId}
                                </p>
                                <Badge tone={googleDriveAccountBadgeTone(account.status)}>
                                  {account.status}
                                </Badge>
                                {savedSelection ? <Badge tone="success">saved selection</Badge> : null}
                                {!savedSelection && selectedInForm ? <Badge tone="neutral">selected in form</Badge> : null}
                              </div>
                              <p className="mt-2 break-all text-xs text-muted-foreground">
                                Account ID: {account.accountId}
                              </p>
                              {account.oauthClientId ? (
                                <p className="mt-1 break-all text-xs text-muted-foreground">
                                  OAuth client: {account.oauthClientId}
                                </p>
                              ) : null}
                            </div>
                          </div>
                          <div className="flex flex-wrap gap-2 lg:pl-0 pl-7">
                            <Badge tone={account.mcpReadReady ? "success" : "warning"}>
                              {account.mcpReadReady ? "proxy read ready" : "proxy read missing scope"}
                            </Badge>
                            <Badge tone={account.mcpWriteReady ? "success" : "warning"}>
                              {account.mcpWriteReady ? "artifact write ready" : "artifact write missing scope"}
                            </Badge>
                            <Badge tone={account.projectCount > 0 ? "success" : "neutral"}>
                              {account.projectCount} project{account.projectCount === 1 ? "" : "s"}
                            </Badge>
                          </div>
                        </div>

                        {isExpanded && (
                          <div className="mt-4 pt-4 border-t border-border/40 transition-all duration-300">
                            <div className="grid gap-2 text-sm text-muted-foreground md:grid-cols-2 pl-7">
                              <p>Granted scopes: {formatGoogleDriveScopeSummary(account.grantedScopes)}</p>
                              <p>Missing scopes: {formatGoogleDriveScopeSummary(account.missingScopes)}</p>
                              <p>Connected at: {account.connectedAt || "unknown"}</p>
                              <p>Updated at: {account.updatedAt || "unknown"}</p>
                            </div>
                            {account.lastError ? (
                              <p className="mt-3 ml-7 rounded-2xl border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-warning">
                                {account.lastError}
                              </p>
                            ) : null}
                            <div className="mt-4 ml-7 flex flex-wrap gap-3">
                              <Button
                                className="w-full md:w-auto"
                                disabled={busyAction !== null}
                                onClick={() => {
                                  void startAccountConnect(account.accountId);
                                }}
                                variant="secondary"
                              >
                                {busyAction === reconnectAction ? "Opening..." : "Reconnect account"}
                              </Button>
                              <Button
                                className="w-full md:w-auto"
                                disabled={busyAction !== null}
                                onClick={() => {
                                  void disconnectAccount(account.accountId);
                                }}
                                variant="secondary"
                              >
                                {busyAction === disconnectAction ? "Disconnecting..." : "Disconnect account"}
                              </Button>
                            </div>
                          </div>
                        )}
                      </div>
                    );
                  })
                )}
              </div>
              <div className="mt-6 flex flex-wrap gap-3">
                <Button onClick={() => setActiveStep(7)} variant="secondary">
                  Next Step
                </Button>
              </div>
            </section>
          )}

          {activeStep === 7 && (
            <section className="rounded-[1.6rem] border border-border bg-background/80 p-6">
              <SectionHeader
                step="7"
                title="Google Drive proxy MCP setup"
                subtitle="Set up the FlowPilot proxy MCP and provider integrations."
                status={status?.mcp.status ?? "not_started"}
              />
              <div className="mt-6 grid gap-4">
                <CollapsibleSetupSection
                  expanded
                  onToggle={() => {}}
                  status={providerSetupStatus}
                  subtitle="Configure Codex, Gemini, and Claude against the FlowPilot proxy MCP."
                  title="Proxy MCP provider setup"
                >
                  <GoogleDriveProviderConfigCard
                    embedded
                    googleDriveStatus={status}
                    onStatusRefresh={refreshStatus}
                  />
                </CollapsibleSetupSection>
              </div>
            </section>
          )}
        </div>
      </div>

      <Dialog open={validationOpen} onOpenChange={setValidationOpen}>
        <DialogContent className="flex h-[min(92vh,58rem)] w-[min(94vw,64rem)] max-w-none flex-col overflow-hidden rounded-[1.75rem] border-border bg-card p-0 shadow-xl">
          <DialogHeader className="border-b border-border/70 px-6 py-5">
            <div className="flex items-start justify-between gap-4 pr-8">
              <div>
                <p className="text-xs uppercase tracking-[0.28em] text-muted-foreground">Step check</p>
                <DialogTitle className="mt-2 text-2xl">Validation results</DialogTitle>
                <DialogDescription className="mt-2 max-w-2xl">
                  The runner returns these checks so you know which Google Console step is still missing.
                </DialogDescription>
              </div>
              <StatusBadge value={validation?.valid ? "configured" : "needs_input"} />
            </div>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
            <div className="grid gap-2">
              {validation?.checks.map((item) => (
                <div key={item.key} className="rounded-2xl border border-border/70 bg-background/70 px-4 py-3 text-sm">
                  <p
                    className={
                      item.status === "passed"
                        ? "text-success"
                        : item.status === "failed"
                          ? "text-danger"
                          : "text-muted-foreground"
                    }
                  >
                    {item.message}
                  </p>
                  <p className="mt-1 text-xs uppercase tracking-[0.22em] text-muted-foreground">
                    {item.key} - {item.status}
                  </p>
                </div>
              )) ?? (
                <p className="rounded-2xl border border-border/70 bg-background/70 px-4 py-3 text-sm text-muted-foreground">
                  Run validation to see the current setup checks.
                </p>
              )}
            </div>
          </div>
          <DialogFooter className="border-t border-border/70 px-6 py-4">
            <Button onClick={() => setValidationOpen(false)} variant="secondary">
              Close
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </PageFrame>
  );
}

function buildStepSummary(
  status: GoogleDriveRuntimeStatus | null,
  step1Manual?: boolean,
  step2Manual?: boolean,
  step3Manual?: boolean,
) {
  const configured = Boolean(
    status?.artifactSync.clientId?.trim() &&
      status?.artifactSync.redirectUri?.trim() &&
      status?.artifactSync.hasClientSecret,
  );
  return {
    project: configured || step1Manual ? "configured" : "not_started",
    consent: configured || step2Manual ? "configured" : "not_started",
    apis: configured || step3Manual ? "configured" : "not_started",
  } as const;
}

function googleDriveAccountBadgeTone(status: string) {
  switch (status) {
    case "configured":
    case "connected":
      return "success";
    case "needs_auth":
    case "needs_input":
    case "reconnect_required":
      return "warning";
    case "failed":
      return "danger";
    default:
      return "neutral";
  }
}

function formatGoogleDriveScopeSummary(scopes?: string[]) {
  return scopes && scopes.length > 0 ? scopes.join(", ") : "none recorded";
}

function providerSetupStepStatus(status: GoogleDriveRuntimeStatus | null) {
  const providerConfigs = status?.providerConfigs ?? [];
  if (providerConfigs.length === 0) {
    return "not_started";
  }
  if (providerConfigs.some((config) => config.status === "failed")) {
    return "failed";
  }
  if (providerConfigs.some((config) => config.status === "config_stale")) {
    return "config_stale";
  }
  if (providerConfigs.every((config) => config.status === "configured")) {
    return "configured";
  }
  return "needs_input";
}

function artifactSyncStepStatus(status?: GoogleDriveRuntimeStatus["artifactSync"] | null) {
  if (!status) {
    return "not_started";
  }
  return Boolean(status.clientId?.trim() && status.redirectUri?.trim() && status.hasClientSecret)
    ? "configured"
    : "needs_input";
}

function StatusStack({ status }: { status: GoogleDriveRuntimeStatus | null }) {
  if (!status) {
    return (
      <div className="rounded-2xl border border-border bg-background/60 px-4 py-3 text-sm">
        Loading Google Drive status...
      </div>
    );
  }

  return (
    <div className="grid gap-3">
      <StatusRow
        label="Artifact sync"
        value={status.artifactSync.status}
        detail={`source: ${status.artifactSync.source}`}
        extra={
          status.artifactSync.configured ? (
            <Link search={{ tab: "storage" }} to="/artifacts">
              <Button className="mt-3 h-auto rounded-full px-3 py-1 text-xs font-medium" variant="secondary">
                Open Artifact Storage Setting
              </Button>
            </Link>
          ) : null
        }
      />
      <StatusRow label="MCP" value={status.mcp.status} detail={googleDriveMcpDetail(status)} />
      <StatusRow
        label="Runner"
        value={status.runnerReachable ? "online" : "offline"}
        detail={status.runnerReachable ? "local runner reachable" : "local runner unavailable"}
      />
    </div>
  );
}

function googleDriveMcpDetail(status: GoogleDriveRuntimeStatus) {
  const accountLabel =
    status.mcp.proxyMcpEnabled && status.mcp.accountId
      ? `account ${status.mcp.accountEmail ? `${status.mcp.accountEmail} ` : ""}(${status.mcp.accountId})`
      : "";
  const missingScopes = status.mcp.missingScopes?.length
    ? `missing scopes: ${status.mcp.missingScopes.join(", ")}`
    : "";

  switch (status.mcp.status) {
    case "configured":
      return accountLabel
        ? `ready · ${accountLabel}`
        : "ready";
    case "reconnect_required":
      return [accountLabel, "reconnect required", missingScopes].filter(Boolean).join(" · ");
    case "needs_auth":
      return [accountLabel, "auth needed", missingScopes].filter(Boolean).join(" · ");
    case "needs_input":
      return status.mcp.proxyMcpEnabled && status.mcp.accountSelectionRequired
        ? "select a Google account"
        : "credentials missing";
    case "failed":
      return [accountLabel, "credentials invalid", missingScopes].filter(Boolean).join(" · ");
    case "warning":
      return status.mcp.backendPackageAvailable ? "warning" : "FlowPilot or Go launcher unavailable";
    default:
      return status.mcp.status || "not started";
  }
}

function StatusRow({
  label,
  value,
  detail,
  extra,
}: {
  label: string;
  value: string;
  detail: string;
  extra?: ReactNode;
}) {
  return (
    <div className="rounded-2xl border border-border/70 bg-background/70 px-4 py-3">
      <div className="flex items-center justify-between gap-4">
        <div>
          <p className="text-sm font-medium">{label}</p>
          <p className="text-xs text-muted-foreground">{detail}</p>
        </div>
        <StatusBadge value={value} />
      </div>
      {extra ? <div>{extra}</div> : null}
    </div>
  );
}

function StatusBadge({ value }: { value: string }) {
  const tone =
    value === "configured" || value === "online"
      ? "border-success/30 bg-success/10 text-success"
      : value === "needs_auth" || value === "needs_input" || value === "reconnect_required"
        ? "border-warning/30 bg-warning/10 text-warning"
        : value === "failed"
          ? "border-danger/30 bg-danger/10 text-danger"
          : "border-border bg-muted text-muted-foreground";
  return <span className={`rounded-full border px-3 py-1 text-xs font-medium uppercase tracking-[0.22em] ${tone}`}>{value}</span>;
}

function SectionHeader({
  step,
  title,
  subtitle,
  status,
}: {
  step: string;
  title: string;
  subtitle: string;
  status: string;
}) {
  return (
    <div className="flex flex-col gap-3 border-b border-border/70 pb-4 md:flex-row md:items-start md:justify-between">
      <div>
        <p className="text-xs uppercase tracking-[0.28em] text-muted-foreground">Step {step}</p>
        <h3 className="mt-1 text-xl font-semibold">{title}</h3>
        <p className="mt-2 text-sm text-muted-foreground">{subtitle}</p>
      </div>
      <StatusBadge value={status} />
    </div>
  );
}

function CollapsibleSetupSection({
  title,
  subtitle,
  status,
  expanded,
  disabled = false,
  onToggle,
  children,
}: {
  title: string;
  subtitle: string;
  status: string;
  expanded: boolean;
  disabled?: boolean;
  onToggle: () => void;
  children: ReactNode;
}) {
  return (
    <div className="overflow-hidden rounded-[1.4rem] border border-border/70 bg-background/60">
      <button
        aria-disabled={disabled}
        className={`flex w-full items-start justify-between gap-4 px-5 py-4 text-left transition-colors ${
          disabled ? "cursor-not-allowed opacity-60" : "hover:bg-muted/20"
        }`}
        disabled={disabled}
        onClick={onToggle}
        type="button"
      >
        <div className="flex min-w-0 items-start gap-3">
          <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-muted">
            {expanded ? (
              <ChevronDown className="h-4 w-4 text-muted-foreground" />
            ) : (
              <ChevronRight className="h-4 w-4 text-muted-foreground" />
            )}
          </div>
          <div className="min-w-0">
            <h4 className="text-lg font-semibold">{title}</h4>
            <p className="mt-1 text-sm text-muted-foreground">{subtitle}</p>
          </div>
        </div>
        <StatusBadge value={status} />
      </button>

      {expanded ? <div className="border-t border-border/70 px-5 py-5">{children}</div> : null}
    </div>
  );
}

function GuideCard({
  index,
  title,
  description,
  items,
  links,
  status,
}: {
  index: string;
  title: string;
  description: string;
  items?: string[];
  links?: Array<{ label: string; href: string }>;
  status: string;
}) {
  return (
    <div className="rounded-2xl border border-border/70 bg-background/70 p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">Step {index}</p>
          <h4 className="mt-1 font-semibold">{title}</h4>
        </div>
        <StatusBadge value={status} />
      </div>
      <p className="mt-3 text-sm text-muted-foreground">{description}</p>
      {items && items.length > 0 ? (
        <GuidePanel className="mt-3" items={items} links={links} />
      ) : null}
    </div>
  );
}

function GuidePanel({
  items,
  links,
  className,
  children,
}: {
  items: string[];
  links?: Array<{ label: string; href: string }>;
  className?: string;
  children?: ReactNode;
}) {
  return (
    <details className={`group rounded-2xl border border-border/70 bg-background/60 p-4 text-sm ${className ?? ""}`}>
      <summary className="flex cursor-pointer list-none items-center justify-between gap-3 font-medium text-foreground">
        <span>Show detailed steps</span>
        <ChevronDown className="h-4 w-4 shrink-0 transition-transform duration-200 group-open:rotate-180" />
      </summary>
      <ol className="mt-3 grid gap-2 text-muted-foreground">
        {items.map((item, index) => (
          <li key={item} className="rounded-xl border border-border/50 bg-background/50 px-3 py-2">
            <span className="mr-2 font-medium text-foreground">{index + 1}.</span>
            {item}
          </li>
        ))}
      </ol>
      {links && links.length > 0 ? (
        <div className="mt-3 grid gap-2">
          {links.map((link) => (
            <a
              key={link.href}
              className="rounded-xl border border-accent/40 bg-accent/10 px-3 py-2 text-sm font-medium text-accent underline underline-offset-4 transition hover:bg-accent/15 hover:text-accent focus:outline-none focus:ring-2 focus:ring-accent/50"
              href={link.href}
              rel="noreferrer"
              target="_blank"
            >
              {link.label}
            </a>
          ))}
        </div>
      ) : null}
      {children ? <div className="mt-3">{children}</div> : null}
    </details>
  );
}

function CodeRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl border border-border/50 bg-background/80 p-3">
      <p className="text-xs uppercase tracking-[0.22em] text-muted-foreground">{label}</p>
      <div className="mt-2 flex overflow-hidden rounded-xl border border-border bg-background">
        <code className="min-w-0 flex-1 overflow-x-auto px-3 py-2 text-xs text-foreground">{value}</code>
        <CopyButton value={value} />
      </div>
    </div>
  );
}

function Field({
  label,
  children,
  className,
}: {
  label: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <label className={`block space-y-2 ${className ?? ""}`}>
      <span className="text-sm text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

function SecretField({
  label,
  note,
  onChange,
  onToggle,
  placeholder,
  reveal,
  value,
}: {
  label: string;
  note?: string;
  onChange: (value: string) => void;
  onToggle: () => void;
  placeholder: string;
  reveal: boolean;
  value: string;
}) {
  return (
    <Field label={label}>
      <div className="flex overflow-hidden rounded-2xl border border-border bg-background focus-within:ring-2 focus-within:ring-accent">
        <input
          className="min-w-0 flex-1 bg-transparent px-4 py-3 outline-none"
          onChange={(event) => onChange(event.target.value)}
          placeholder={placeholder}
          type={reveal ? "text" : "password"}
          value={value}
        />
        <button className="px-4 text-muted-foreground hover:text-foreground" onClick={onToggle} type="button">
          {reveal ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </button>
      </div>
      {note ? <p className="text-xs text-muted-foreground">{note}</p> : null}
    </Field>
  );
}

function CopyButton({ value }: { value: string }) {
  return (
    <button
      className="inline-flex items-center gap-2 border-l border-border px-4 text-xs font-medium text-muted-foreground hover:bg-muted/70 hover:text-foreground"
      onClick={async () => {
        if (!value.trim()) {
          return;
        }
        await navigator.clipboard.writeText(value);
      }}
      type="button"
    >
      <Copy className="h-4 w-4" />
      Copy
    </button>
  );
}

function SetupDetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl border border-border/50 bg-background/80 p-3">
      <p className="text-xs uppercase tracking-[0.22em] text-muted-foreground">{label}</p>
      <p className="mt-2 break-words text-sm font-medium text-foreground">{value}</p>
    </div>
  );
}

async function submitGoogleDriveConfig(
  path: string,
  payload: Record<string, string>,
  method = "POST",
) {
  const response = await fetch(path, {
    method,
    headers: { "content-type": "application/json" },
    body: JSON.stringify(payload),
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(body.error ?? "Google Drive config request failed.");
  }
  return body as GoogleDriveValidationResult | GoogleDriveRuntimeStatus;
}
