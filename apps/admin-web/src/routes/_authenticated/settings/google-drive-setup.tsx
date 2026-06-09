import { useMutation } from "@tanstack/react-query";
import { createFileRoute, Link, useRouter } from "@tanstack/react-router";
import { ChevronDown, ChevronRight, Copy, Eye, EyeOff, RefreshCw } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type DragEvent, type KeyboardEvent, type ReactNode } from "react";

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
import { createGatewayBundle } from "@/data/repository/browser-factory";
import type { IntegrationType } from "@/domain/model/entity/integration";
import type { LocalRunnerMcpBackend } from "@/domain/model/entity/local-runner";
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

export const Route = createFileRoute("/_authenticated/settings/google-drive-setup")({
  loader: loadMcpSettingsData,
  component: GoogleDriveSetupPage,
});

function GoogleDriveSetupPage() {
  const { allIntegrations, backends, health, projects } = Route.useLoaderData();
  const router = useRouter();
  const [status, setStatus] = useState<GoogleDriveRuntimeStatus | null>(null);
  const [clientIdEditable, setClientIdEditable] = useState(false);
  const oauthFileInputRef = useRef<HTMLInputElement | null>(null);
  const [artifactForm, setArtifactForm] = useState({
    clientId: "",
    clientSecret: "",
    redirectUri: DEFAULT_REDIRECT_URI,
  });
  const [pickerApiKey, setPickerApiKey] = useState("");
  const [oauthFile, setOauthFile] = useState<File | null>(null);
  const [showClientSecret, setShowClientSecret] = useState(false);
  const [showPickerApiKey, setShowPickerApiKey] = useState(false);
  const [expandedStep6Sections, setExpandedStep6Sections] = useState<Record<"6.1" | "6.2" | "6.3", boolean>>({
    "6.1": true,
    "6.2": true,
    "6.3": true,
  });
  const [validation, setValidation] = useState<GoogleDriveValidationResult | null>(null);
  const [validationOpen, setValidationOpen] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [busyAction, setBusyAction] = useState<string | null>(null);
  const runnerOnline = health.status === "online";
  const googleDriveBackend = backends.find((backend) => backend.providerType === "google_drive") ?? null;
  const googleDriveIntegrations = allIntegrations.filter(
    (integration) => integration.type === "google_drive",
  );
  const googleDriveTypeEnabled =
    googleDriveBackend?.transport === "remote"
      ? googleDriveIntegrations.some((integration) => integration.mcpTypeEnabled === true)
      : googleDriveBackend?.state === "installed";

  useEffect(() => {
    let active = true;
    void loadGoogleDriveRuntimeStatus()
      .then((nextStatus) => {
        if (!active) {
          return;
        }
        setStatus(nextStatus);
        setArtifactForm({
          clientId: nextStatus.artifactSync.clientId ?? "",
          clientSecret: "",
          redirectUri: nextStatus.artifactSync.redirectUri ?? DEFAULT_REDIRECT_URI,
        });
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

  const stepSummary = useMemo(() => buildStepSummary(status), [status]);
  const proxyMcpEnabled = Boolean(status?.mcp.proxyMcpEnabled);
  const canSaveArtifactSync = artifactForm.clientId.trim().length > 0;
  const canSavePickerApiKey = pickerApiKey.trim().length > 0;
  const providerSetupStatus = providerSetupStepStatus(status);
  const mcpLaunchStatus = backendSetupStepStatus(googleDriveBackend, googleDriveTypeEnabled);

  useEffect(() => {
    if (!proxyMcpEnabled) {
      return;
    }
    setExpandedStep6Sections((current) => ({
      ...current,
      "6.1": false,
      "6.2": false,
    }));
  }, [proxyMcpEnabled]);

  function toggleStep6Section(section: "6.1" | "6.2" | "6.3") {
    if (proxyMcpEnabled && (section === "6.1" || section === "6.2")) {
      return;
    }
    setExpandedStep6Sections((current) => ({
      ...current,
      [section]: !current[section],
    }));
  }

  const runBackendAction = useMutation({
    mutationFn: async (backend: LocalRunnerMcpBackend) => {
      if (!runnerOnline) {
        throw new Error(`The local runner is unreachable at ${health.baseUrl}.`);
      }

      const gateways = await createGatewayBundle();
      if (backend.action === "install") {
        return gateways.localRunnerGateway.installMcpBackend(backend.key);
      }
      return gateways.localRunnerGateway.triggerMcpBackendAction(backend.key, {
        projectId: googleDriveIntegrations[0]?.projectId ?? projects[0]?.id ?? "",
        action: backend.action,
      });
    },
    onSuccess: async () => {
      await router.invalidate();
    },
    onError: (error) => {
      setMessage(error instanceof Error ? error.message : "Unable to manage Google Drive MCP.");
    },
  });

  const toggleMcpType = useMutation({
    mutationFn: async ({ enabled, providerType }: { enabled: boolean; providerType: IntegrationType }) => {
      const gateways = await createGatewayBundle();
      await Promise.all(
        googleDriveIntegrations.map((integration) =>
          gateways.integrationGateway.updateIntegration(integration.id, {
            type: integration.type,
            label: integration.label,
            configEncrypted: integration.configEncrypted,
            status: integration.status,
            lastSyncedAt: integration.lastSyncedAt,
            lastError: integration.lastError,
            mcpTypeEnabled: enabled,
          }),
        ),
      );

      if (enabled || googleDriveIntegrations.length > 0) {
        return;
      }

      await router.navigate({
        to: "/settings/mcp-servers/create",
        search: { provider: providerType },
      });
    },
    onSuccess: async () => {
      await router.invalidate();
    },
    onError: (error) => {
      setMessage(error instanceof Error ? error.message : "Unable to update Google Drive MCP type.");
    },
  });

  const refreshStatus = async () => {
    const nextStatus = await loadGoogleDriveRuntimeStatus();
    setStatus(nextStatus);
    return nextStatus;
  };

  const validateSetup = async () => {
    setBusyAction("validate");
    setMessage(null);
    try {
      const result = (await submitGoogleDriveConfig(
        "/api/runtime/google-drive-config/validate",
        {
          clientId: artifactForm.clientId,
          clientSecret: artifactForm.clientSecret,
          redirectUri: artifactForm.redirectUri,
          pickerApiKey,
        },
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
      await submitGoogleDriveConfig(
        "/api/runtime/google-drive-config",
        {
          clientId: artifactForm.clientId,
          clientSecret: artifactForm.clientSecret,
          redirectUri: artifactForm.redirectUri,
        },
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

  const uploadMcpOAuthJson = async (file = oauthFile) => {
    if (!file) {
      setMessage("Choose the legacy Desktop OAuth JSON file first.");
      return;
    }
    setBusyAction("upload-mcp");
    setMessage(null);
    try {
      const formData = new FormData();
      formData.append("file", file);
      const response = await fetch("/api/runtime/google-drive-config/mcp-oauth-upload", {
        method: "POST",
        body: formData,
      });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(payload.error ?? "Unable to upload legacy Desktop OAuth JSON.");
      }
      setStatus(payload as GoogleDriveRuntimeStatus);
      setMessage("OAuth JSON uploaded to the runner-managed legacy MCP config path.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to upload legacy Desktop OAuth JSON.");
    } finally {
      setBusyAction(null);
    }
  };

  const saveMcpOAuthJson = async () => {
    await uploadMcpOAuthJson();
  };

  const selectMcpOAuthFile = (file: File | null) => {
    setOauthFile(file);
    setMessage(null);
  };

  const openMcpOAuthPicker = () => {
    oauthFileInputRef.current?.click();
  };

  const handleMcpOAuthDrop = (event: DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    const file = event.dataTransfer.files?.[0] ?? null;
    selectMcpOAuthFile(file);
  };

  const handleMcpOAuthKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      openMcpOAuthPicker();
    }
  };

  const refreshMcpStatus = async () => {
    setBusyAction("refresh-mcp");
    setMessage(null);
    try {
      const nextStatus = await refreshStatus();
      setMessage(
        nextStatus.mcp.needsAuth
          ? "Legacy raw MCP still needs auth. Finish the auth flow, then refresh again."
          : "Legacy raw MCP status refreshed.",
      );
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to refresh legacy raw MCP status.");
    } finally {
      setBusyAction(null);
    }
  };

  const startMcpAuth = async () => {
    setBusyAction("start-mcp-auth");
    setMessage(null);
    try {
      const response = await fetch("/api/runtime/google-drive-config/mcp-auth/start", {
        method: "POST",
      });
      const payload = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(payload.error ?? "Unable to start legacy raw MCP auth.");
      }
      setStatus((payload.config as GoogleDriveRuntimeStatus) ?? status);
      setMessage(
        payload.message ??
          "Legacy raw MCP auth opened in a new terminal. Complete sign-in, then refresh legacy status.",
      );
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to start legacy raw MCP auth.");
    } finally {
      setBusyAction(null);
    }
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
      const nextStatus = await refreshStatus();
      setValidation(null);
      setMessage(`Google Drive config reset. Current mode: ${nextStatus.artifactSync.source}.`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Unable to reset Google Drive config.");
    } finally {
      setBusyAction(null);
    }
  };

  return (
    <PageFrame
      title="Google Console Setup"
      description="Save the Google Cloud values once, then let FlowPilot reuse them for artifact sync and the proxy MCP."
    >
      <section className="rounded-[1.6rem] border border-border bg-card/80 p-6">
        <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
          <div>
            <p className="text-xs uppercase tracking-[0.28em] text-muted-foreground">Setup validation</p>
            <h2 className="mt-2 text-2xl font-semibold">Validate current Google Console setup</h2>
            <p className="mt-3 max-w-2xl text-sm text-muted-foreground">
              Run the runner-side checks before saving or after updating credentials. The result opens in
              a modal so you can review every passed and failed item without pushing the form sections
              down the page.
            </p>
          </div>
          <div className="flex flex-col items-start gap-3 md:items-end">
            <Button disabled={busyAction === "validate"} onClick={validateSetup} variant="secondary">
              {busyAction === "validate" ? "Validating..." : "Validate setup"}
            </Button>
            {validation ? (
              <button
                className="transition hover:opacity-85"
                onClick={() => setValidationOpen(true)}
                type="button"
                aria-label="Open last validation result"
              >
                <div className="flex items-center gap-2">
                  <span className="text-xs uppercase tracking-[0.22em] text-muted-foreground">
                    Latest check
                  </span>
                  <StatusBadge value={validation.valid ? "configured" : "needs_input"} />
                </div>
              </button>
            ) : null}
          </div>
        </div>
      </section>

      <div className="grid gap-6 lg:grid-cols-[1.2fr_0.8fr]">
        <section className="rounded-[1.6rem] border border-border bg-background/80 p-6">
          <p className="text-xs uppercase tracking-[0.28em] text-muted-foreground">Google Cloud Console</p>
          <h2 className="mt-2 text-2xl font-semibold">Manual in Google Project Console, automatic handle values in FlowPilot</h2>
          <p className="mt-3 text-sm text-muted-foreground">
            Create the Google project, OAuth clients, and API key in Google Cloud Console first. After
            that, FlowPilot stores the local values and keeps artifact sync plus MCP pointed at the same
            project.
          </p>

          <div className="mt-5 grid gap-4">
            <GuideCard
              index="1"
              title="Create project"
              description="Use one Google Cloud project for this MVP and keep the active project selected."
              items={[
                "Open Google Cloud Console.",
                "Use the top project selector.",
                "Create one project such as FlowPilot Local if needed.",
                "Make sure that project is the active one before creating clients or keys.",
              ]}
              status={stepSummary.project}
            />
            <GuideCard
              index="2"
              title="Configure OAuth consent screen"
              description="Use External for personal testing, add test users, and switch to Production when you are done testing."
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
              status={stepSummary.consent}
            />
            <GuideCard
              index="3"
              title="Enable APIs"
              description="Enable Google Drive API, Google Picker API, and the Docs/Sheets/Slides APIs if the MCP tools need them."
              items={[
                "Open APIs & Services > Library.",
                "Enable Google Drive API.",
                "Enable Google Picker API.",
                "Also enable Google Docs API, Google Sheets API, and Google Slides API if you want broader MCP support.",
              ]}
              status={stepSummary.apis}
            />
          </div>
        </section>

        <section className="rounded-[1.6rem] border border-border bg-card/80 p-6">
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
        </section>
      </div>

      <div className="mt-6 grid gap-6">
        <section className="rounded-[1.6rem] border border-border bg-background/80 p-6">
          <SectionHeader
            step="4"
            title="Create Web OAuth client for artifact sync"
            subtitle="Save the Web OAuth client locally so artifact sync can start and refresh without editing .env."
            status={artifactSyncStepStatus(status?.artifactSync)}
          />
          <GuidePanel
            className="mt-4"
            items={[
              "Open Google Auth Platform > Clients.",
              "Click Create client.",
              "Choose Web application.",
              "Use a clear name such as FlowPilot Artifact Sync Local.",
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
          </div>
        </section>

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
            <div className="flex items-end">
              <Button
                disabled={busyAction === "save-picker" || !canSavePickerApiKey}
                onClick={savePickerApiKey}
                className="w-full md:w-auto"
              >
                {busyAction === "save-picker" ? "Saving..." : "Save picker key"}
              </Button>
            </div>
          </div>
        </section>

        <section className="rounded-[1.6rem] border border-border bg-background/80 p-6">
          <SectionHeader
            step="6"
            title="Google Drive proxy MCP setup"
            subtitle="Set up the FlowPilot proxy MCP first. Use the Desktop OAuth JSON flow only for legacy raw-MCP fallback."
            status={status?.mcp.status ?? "not_started"}
          />
          <div className="mt-6 grid gap-4">
            <CollapsibleSetupSection
              disabled={proxyMcpEnabled}
              expanded={!proxyMcpEnabled && expandedStep6Sections["6.1"]}
              onToggle={() => toggleStep6Section("6.1")}
              status={status?.mcp.status ?? "not_started"}
              subtitle={
                proxyMcpEnabled
                  ? "Disabled while FlowPilot proxy MCP is enabled on this runner."
                  : "Only upload Desktop OAuth JSON if you still need the legacy raw-MCP fallback."
              }
              title="6.1 Legacy raw MCP Desktop OAuth fallback"
            >
              <GuidePanel
                className="mt-4"
                items={[
                  "Open Google Auth Platform > Clients.",
                  "Click Create client.",
                  "Choose Desktop app.",
                  "Use a clear name such as FlowPilot Google Drive MCP Legacy.",
                  "Save the client and download the OAuth JSON file.",
                  "Choose or Drag the Desktop client JSON here only for legacy raw-MCP fallback. The proxy MCP path does not need this file.",
                ]}
              />
              <div className="mt-4 grid gap-4 lg:grid-cols-[1.2fr_0.8fr]">
                <div className="grid gap-4">
                  <div className="space-y-2">
                    <span className="text-sm text-muted-foreground">
                      <span className="text-accent">Choose or Drag legacy file to upload</span>
                    </span>
                    <div
                      aria-label="Choose or Drag legacy file to upload"
                      className="flex min-h-14 cursor-pointer items-center overflow-hidden rounded-2xl border border-border bg-background text-sm outline-none transition hover:border-accent/50 focus-within:border-accent focus-within:ring-2 focus-within:ring-accent/40"
                      onClick={openMcpOAuthPicker}
                      onDrop={handleMcpOAuthDrop}
                      onDragOver={(event) => event.preventDefault()}
                      onKeyDown={handleMcpOAuthKeyDown}
                      role="button"
                      tabIndex={0}
                    >
                      <input
                        ref={oauthFileInputRef}
                        accept="application/json,.json"
                        className="sr-only"
                        onChange={(event) => {
                          const file = event.target.files?.[0] ?? null;
                          selectMcpOAuthFile(file);
                        }}
                        type="file"
                      />
                      <span className={`min-w-0 flex-1 px-4 py-3 ${oauthFile ? "text-foreground" : "text-muted-foreground"}`}>
                        {oauthFile ? oauthFile.name : "Drop the legacy Desktop OAuth JSON here or click to browse"}
                      </span>
                    </div>
                  </div>
                  <div className="rounded-2xl border border-border/70 bg-background/60 p-4 text-sm text-muted-foreground">
                    <p className="font-medium text-foreground">Legacy raw-MCP paths</p>
                    <p className="mt-2 break-all">Credentials: {status?.mcp.credentialPath ?? "Not resolved yet"}</p>
                    <p className="mt-1 break-all">Token: {status?.mcp.tokenPath ?? "Not resolved yet"}</p>
                    <p className="mt-1">
                      Token file exists: {status?.mcp.tokenFileExists ? "yes" : "no"} | Auth required:{" "}
                      {status?.mcp.needsAuth ? "yes" : "no"}
                    </p>
                  </div>
                </div>
                <div className="rounded-2xl border border-border/70 bg-background/60 p-4 text-sm">
                  <p className="font-medium text-foreground">What this does</p>
                  <ul className="mt-3 grid gap-2 text-muted-foreground">
                    <li>Validates the downloaded Google Desktop OAuth JSON for the legacy raw-MCP package.</li>
                    <li>Copies it to `~/.config/google-drive-mcp/gcp-oauth.keys.json`.</li>
                    <li>Leaves the token file for the legacy MCP auth flow to create later.</li>
                    <li>Does not affect proxy MCP readiness.</li>
                  </ul>
                </div>
              </div>
              <div className="mt-5 flex flex-wrap gap-3">
                <Button disabled={busyAction === "upload-mcp" || !oauthFile} onClick={saveMcpOAuthJson}>
                  {busyAction === "upload-mcp" ? "Saving..." : "Save legacy OAuth JSON"}
                </Button>
                <Button disabled={busyAction === "refresh-mcp"} onClick={refreshMcpStatus} variant="secondary">
                  <RefreshCw className="mr-2 h-4 w-4" />
                  {busyAction === "refresh-mcp" ? "Refreshing..." : "Refresh legacy MCP status"}
                </Button>
              </div>
            </CollapsibleSetupSection>

            <CollapsibleSetupSection
              disabled={proxyMcpEnabled}
              expanded={!proxyMcpEnabled && expandedStep6Sections["6.2"]}
              onToggle={() => toggleStep6Section("6.2")}
              status={mcpLaunchStatus}
              subtitle={
                proxyMcpEnabled
                  ? "Disabled while FlowPilot proxy MCP is enabled on this runner."
                  : "Install, launch, and authenticate the third-party MCP package only when you need the legacy fallback."
              }
              title="6.2 Legacy raw MCP launch"
            >
              <div className="rounded-[1.6rem] border border-border/70 bg-card/50 p-5">
                <div className="flex flex-wrap items-start justify-between gap-4">
                  <div>
                    <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
                      Legacy raw MCP
                    </p>
                    <h4 className="mt-2 text-xl font-semibold tracking-tight">
                      Backend status moved here from MCP Servers
                    </h4>
                    <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
                      This section only applies to the legacy raw-MCP package. The FlowPilot proxy MCP reuses the artifact-sync Google OAuth connection instead.
                    </p>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Link
                      className="inline-flex items-center justify-center rounded-full border border-border bg-background px-4 py-2 text-sm font-semibold transition-colors hover:bg-muted"
                      search={{ provider: "google_drive" }}
                      to="/settings/mcp-servers/create"
                    >
                      Create MCP
                    </Link>
                    <Link
                      className="inline-flex items-center justify-center rounded-full border border-border bg-background px-4 py-2 text-sm font-semibold transition-colors hover:bg-muted"
                      to="/settings/mcp-servers/jira-link"
                    >
                      Jira MCP Link
                    </Link>
                  </div>
                </div>

                {googleDriveBackend ? (
                  <div className="mt-5 rounded-[1.4rem] border border-border bg-background/70 p-5">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <p className="font-mono text-xs uppercase tracking-[0.24em] text-muted-foreground">
                          {googleDriveBackend.providerType}
                        </p>
                        <h5 className="mt-2 text-lg font-semibold tracking-tight">{googleDriveBackend.label}</h5>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        <Badge tone={backendTone(googleDriveBackend.state)}>
                          {backendStateLabel(googleDriveBackend.state)}
                        </Badge>
                        <Badge tone={googleDriveTypeEnabled ? "success" : "neutral"}>
                          {googleDriveTypeEnabled ? "enabled" : "disabled"}
                        </Badge>
                        <Badge tone="neutral">{googleDriveBackend.transport}</Badge>
                      </div>
                    </div>

                    <div className="mt-5 grid gap-3 sm:grid-cols-2">
                      <SetupDetailRow label="Launcher" value={googleDriveBackend.launcher} />
                      <SetupDetailRow label="Command" value={googleDriveBackend.command} />
                      <SetupDetailRow
                        label="Type State"
                        value={
                          googleDriveTypeEnabled
                            ? "Enabled for Google Drive MCP instances"
                            : "Disabled until enabled"
                        }
                      />
                      <SetupDetailRow
                        label="Last Checked"
                        value={googleDriveBackend.lastCheckedAt ? new Date(googleDriveBackend.lastCheckedAt).toLocaleString() : "Never"}
                      />
                    </div>

                    {googleDriveBackend.lastError ? (
                      <p className="mt-4 rounded-2xl border border-danger/30 bg-danger/10 px-4 py-3 text-sm text-danger">
                        Last error: {googleDriveBackend.lastError}
                      </p>
                    ) : null}

                    <div className="mt-4 flex flex-wrap gap-2">
                      <Button
                        disabled={!runnerOnline || runBackendAction.isPending}
                        onClick={() => runBackendAction.mutate(googleDriveBackend)}
                        type="button"
                        variant="secondary"
                      >
                        {runBackendAction.isPending ? "Working..." : googleDriveBackend.actionLabel}
                      </Button>
                      {googleDriveBackend.transport === "launcher" ? (
                        <Button
                          disabled={
                            !runnerOnline ||
                            busyAction === "start-mcp-auth" ||
                            !status?.mcp.credentialFileValid
                          }
                          onClick={startMcpAuth}
                          type="button"
                          variant="secondary"
                        >
                          {busyAction === "start-mcp-auth" ? "Opening..." : "Start Auth"}
                        </Button>
                      ) : null}
                      {googleDriveBackend.transport === "remote" ? (
                        googleDriveTypeEnabled ? (
                          <Button
                            className="bg-danger text-white hover:bg-danger/90"
                            disabled={!runnerOnline || toggleMcpType.isPending}
                            onClick={() =>
                              toggleMcpType.mutate({ enabled: false, providerType: "google_drive" })
                            }
                            type="button"
                            variant="secondary"
                          >
                            {toggleMcpType.isPending ? "Working..." : "Disable"}
                          </Button>
                        ) : (
                          <Button
                            disabled={!runnerOnline || toggleMcpType.isPending}
                            onClick={() =>
                              toggleMcpType.mutate({ enabled: true, providerType: "google_drive" })
                            }
                            type="button"
                            variant="secondary"
                          >
                            {toggleMcpType.isPending ? "Working..." : "Enable"}
                          </Button>
                        )
                      ) : null}
                    </div>
                  </div>
                ) : (
                  <div className="mt-5 rounded-2xl border border-dashed border-border bg-background/60 p-4 text-sm text-muted-foreground">
                    No Google Drive MCP backend was returned by the local runner.
                  </div>
                )}
              </div>
            </CollapsibleSetupSection>

            <CollapsibleSetupSection
              expanded={expandedStep6Sections["6.3"]}
              onToggle={() => toggleStep6Section("6.3")}
              status={providerSetupStatus}
              subtitle="Configure Codex, Gemini, and Claude against the FlowPilot proxy MCP. Keep the legacy raw-MCP/Desktop OAuth path only if you still need it."
              title="6.3 Proxy MCP provider setup"
            >
              <GoogleDriveProviderConfigCard
                embedded
                googleDriveStatus={status}
                onStatusRefresh={refreshStatus}
              />
            </CollapsibleSetupSection>
          </div>
        </section>
      </div>

      <div className="mt-6 flex flex-wrap gap-3">
        <Button disabled={busyAction === "reset"} onClick={resetConfig} variant="secondary">
          {busyAction === "reset" ? "Resetting..." : "Reset Google Drive config"}
        </Button>
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

function buildStepSummary(status: GoogleDriveRuntimeStatus | null) {
  const configured = Boolean(
    status?.artifactSync.clientId?.trim() &&
      status?.artifactSync.redirectUri?.trim() &&
      status?.artifactSync.hasClientSecret,
  );
  return {
    project: configured ? "configured" : "not_started",
    consent: configured ? "configured" : "not_started",
    apis: configured ? "configured" : "not_started",
  } as const;
}

function backendTone(state: LocalRunnerMcpBackend["state"]) {
  switch (state) {
    case "installed":
      return "success";
    case "launcher_available":
      return "warning";
    case "missing":
      return "danger";
  }
}

function backendStateLabel(state: LocalRunnerMcpBackend["state"]) {
  switch (state) {
    case "launcher_available":
      return "Launcher Available";
    default:
      return state.replace("_", " ");
  }
}

function backendSetupStepStatus(
  backend: LocalRunnerMcpBackend | null,
  googleDriveTypeEnabled: boolean,
) {
  if (!backend) {
    return "not_started";
  }
  if (backend.lastError) {
    return "failed";
  }
  if (backend.transport === "launcher") {
    return backend.state === "installed" ? "configured" : "needs_input";
  }
  return googleDriveTypeEnabled ? "configured" : "needs_input";
}

function providerSetupStepStatus(status: GoogleDriveRuntimeStatus | null) {
  const providerConfigs = status?.providerConfigs ?? [];
  if (providerConfigs.length === 0) {
    return "not_started";
  }
  if (providerConfigs.some((config) => config.status === "failed")) {
    return "failed";
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
  switch (status.mcp.status) {
    case "configured":
      return "ready";
    case "reconnect_required":
      return "reconnect required";
    case "needs_auth":
      return "auth needed";
    case "needs_input":
      return "credentials missing";
    case "failed":
      return "credentials invalid";
    case "warning":
      return "backend unavailable";
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
