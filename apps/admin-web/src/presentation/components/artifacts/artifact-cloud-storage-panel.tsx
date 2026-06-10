"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Check, FolderOpen, Link2, Loader2, RefreshCw } from "lucide-react";
import { useRouter } from "next/navigation";

import { getBrowserSupabaseClient } from "@/data/supabase/client";
import type { Project } from "@/domain/model/entity/project";
import {
  loadGoogleDriveRuntimeStatus,
  type GoogleDriveRuntimeStatus,
} from "@/lib/google-drive/runtime-config";
import { Badge } from "@/presentation/components/ui/badge";
import { Button } from "@/presentation/components/ui/button";

type GoogleDriveConnectionPayload = {
  connection: {
    projectId: string;
    status: string;
    folderId?: string;
    folderName?: string;
    accountId?: string;
    accountEmail?: string;
    lastError?: string;
    lastValidatedAt?: string;
    connectedAt?: string;
  };
  session?: {
    sessionId: string;
    status: string;
    connectUrl?: string;
    expiresAt?: string;
    accountId?: string;
    accountEmail?: string;
    lastError?: string;
  } | null;
};

interface ArtifactCloudStoragePanelProps {
  projects: Project[];
}

type StorageProviderOption = "supabase" | "google_drive";
type ProviderSwitchStage =
  | "validating_target_provider"
  | "scanning_artifacts"
  | "checking_existing_replicas"
  | "syncing"
  | "completed"
  | "failed"
  | "reconnect_required";
type ProviderSwitchResult = {
  status: "completed" | "failed";
  sourceProvider: StorageProviderOption;
  targetProvider: StorageProviderOption;
  totalArtifacts: number;
  syncedCount: number;
  skippedCount: number;
  failedCount: number;
  failures: Array<{
    artifactRunId: string;
    reason: string;
  }>;
};
type ProviderSwitchState = {
  targetProvider: StorageProviderOption;
  stage: ProviderSwitchStage;
  message: string;
  totalArtifacts: number;
  syncedCount: number;
  skippedCount: number;
  failedCount: number;
  currentArtifactLabel?: string | null;
  failures: ProviderSwitchResult["failures"];
};

export function ArtifactCloudStoragePanel({ projects }: ArtifactCloudStoragePanelProps) {
  const router = useRouter();
  const [projectList, setProjectList] = useState(projects);
  const [selectedProjectId, setSelectedProjectId] = useState(projects[0]?.id ?? "");
  const [detailProvider, setDetailProvider] = useState<StorageProviderOption>("supabase");
  const [googleDriveState, setGoogleDriveState] = useState<GoogleDriveConnectionPayload | null>(null);
  const [googleDriveSetupStatus, setGoogleDriveSetupStatus] = useState<GoogleDriveRuntimeStatus | null>(null);
  const [selectedGoogleDriveAccountId, setSelectedGoogleDriveAccountId] = useState("");
  const [loadingState, setLoadingState] = useState(false);
  const [loadingSetupState, setLoadingSetupState] = useState(false);
  const [startingSession, setStartingSession] = useState(false);
  const [savingProvider, setSavingProvider] = useState<"supabase" | "google_drive" | null>(null);
  const [providerSwitchState, setProviderSwitchState] = useState<ProviderSwitchState | null>(null);
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const pollingTimerRef = useRef<number | null>(null);
  const switchProgressTimersRef = useRef<number[]>([]);
  const autoProviderSwitchKeyRef = useRef<string | null>(null);

  const selectedProject = useMemo(
    () => projectList.find((project) => project.id === selectedProjectId) ?? null,
    [projectList, selectedProjectId],
  );

  useEffect(() => {
    setProjectList(projects);
    setSelectedProjectId((current) =>
      projects.some((project) => project.id === current) ? current : (projects[0]?.id ?? ""),
    );
  }, [projects]);

  async function buildAuthHeaders() {
    const supabase = await getBrowserSupabaseClient();
    const token = (await supabase.auth.getSession()).data.session?.access_token ?? "";
    const headers: Record<string, string> = {};
    if (token) {
      headers.Authorization = `Bearer ${token}`;
    }
    return headers;
  }

  async function loadGoogleDriveState(sessionId?: string) {
    if (!selectedProjectId) {
      setGoogleDriveState(null);
      return;
    }
    setLoadingState(true);
    try {
      const url = new URL("/api/local-runner/artifact-storage/google-drive/status", window.location.origin);
      url.searchParams.set("projectId", selectedProjectId);
      if (sessionId) {
        url.searchParams.set("sessionId", sessionId);
      }
      const response = await fetch(url, {
        cache: "no-store",
        headers: await buildAuthHeaders(),
      });
      if (response.status === 401) {
        router.replace("/login");
        return;
      }
      const payload = (await response.json()) as GoogleDriveConnectionPayload | { error?: string };
      if (!response.ok) {
        if ("error" in payload && payload.error === "Authentication required.") {
          router.replace("/login");
          return;
        }
        throw new Error("error" in payload && payload.error ? payload.error : "Unable to read Google Drive status.");
      }
      const nextState = payload as GoogleDriveConnectionPayload;
      setGoogleDriveState(nextState);
      const session = nextState.session;
      if (session && (session.status === "pending" || session.status === "awaiting_oauth" || session.status === "awaiting_folder_selection")) {
        schedulePolling(session.sessionId);
      } else {
        stopPolling();
      }
      const connection = nextState.connection;
      const shouldActivateGoogleDrive =
        Boolean(sessionId) &&
        connection?.status === "connected" &&
        Boolean(connection.folderId?.trim()) &&
        selectedProject?.artifactStoragePreference !== "google_drive" &&
        savingProvider === null;
      const autoSwitchKey = `${selectedProjectId}:${connection?.folderId ?? ""}`;
      if (shouldActivateGoogleDrive && autoProviderSwitchKeyRef.current !== autoSwitchKey) {
        autoProviderSwitchKeyRef.current = autoSwitchKey;
        setStatusMessage("Google Drive folder connected. Activating Google Drive as this project's artifact provider...");
        await selectProvider("google_drive");
      }
    } catch (error) {
      setStatusMessage(error instanceof Error ? error.message : "Unable to read Google Drive status.");
      stopPolling();
    } finally {
      setLoadingState(false);
    }
  }

  async function loadGoogleDriveSetup() {
    setLoadingSetupState(true);
    try {
      setGoogleDriveSetupStatus(await loadGoogleDriveRuntimeStatus());
    } catch (error) {
      setStatusMessage(error instanceof Error ? error.message : "Unable to read Google Drive setup status.");
    } finally {
      setLoadingSetupState(false);
    }
  }

  function stopPolling() {
    if (pollingTimerRef.current !== null) {
      window.clearTimeout(pollingTimerRef.current);
      pollingTimerRef.current = null;
    }
  }

  function clearSwitchProgressTimers() {
    for (const timerId of switchProgressTimersRef.current) {
      window.clearTimeout(timerId);
    }
    switchProgressTimersRef.current = [];
  }

  function updateProviderSwitchState(stage: ProviderSwitchStage, message: string) {
    setProviderSwitchState((current) =>
      current
        ? {
            ...current,
            stage,
            message,
          }
        : current,
    );
  }

  function startProviderSwitchProgress(targetProvider: StorageProviderOption) {
    clearSwitchProgressTimers();
    setProviderSwitchState({
      targetProvider,
      stage: "validating_target_provider",
      message: `Validating ${targetProvider === "google_drive" ? "Google Drive" : "Supabase"} target provider...`,
      totalArtifacts: 0,
      syncedCount: 0,
      skippedCount: 0,
      failedCount: 0,
      currentArtifactLabel: null,
      failures: [],
    });

    const checkpoints: Array<{ delayMs: number; stage: ProviderSwitchStage; message: string }> = [
      {
        delayMs: 250,
        stage: "scanning_artifacts",
        message: "Scanning local artifacts, shared artifact rows, and provider replicas...",
      },
      {
        delayMs: 900,
        stage: "checking_existing_replicas",
        message: "Checking existing target replicas before downloading remote bytes...",
      },
      {
        delayMs: 1600,
        stage: "syncing",
        message: "Syncing missing artifacts into the selected target provider...",
      },
    ];

    switchProgressTimersRef.current = checkpoints.map(({ delayMs, stage, message }) =>
      window.setTimeout(() => {
        updateProviderSwitchState(stage, message);
      }, delayMs),
    );
  }

  function schedulePolling(sessionId: string) {
    stopPolling();
    pollingTimerRef.current = window.setTimeout(() => {
      void loadGoogleDriveState(sessionId);
    }, 2000);
  }

  async function startGoogleDriveConnect() {
    if (!selectedProjectId || !selectedGoogleDriveAccountId.trim()) {
      return;
    }
    setStartingSession(true);
    setStatusMessage(null);
    try {
      const response = await fetch("/api/local-runner/artifact-storage/google-drive/connect-session", {
        method: "POST",
        headers: {
          "content-type": "application/json",
          ...(await buildAuthHeaders()),
        },
        body: JSON.stringify({
          projectId: selectedProjectId,
          accountId: selectedGoogleDriveAccountId.trim(),
        }),
      });
      if (response.status === 401) {
        router.replace("/login");
        return;
      }
      const payload = (await response.json()) as
        | ({ error?: string } & Partial<GoogleDriveConnectionPayload["session"]>)
        | { error?: string };
      if (!response.ok) {
        if ("error" in payload && payload.error === "Authentication required.") {
          router.replace("/login");
          return;
        }
        throw new Error("error" in payload && payload.error ? payload.error : "Unable to start Google Drive connection.");
      }

      const session = payload as NonNullable<GoogleDriveConnectionPayload["session"]>;
      setGoogleDriveState((current) => ({
        connection:
          current?.connection ?? {
            projectId: selectedProjectId,
            status: "pending",
          },
        session,
      }));
      if (session.connectUrl) {
        window.open(session.connectUrl, "_blank", "noopener,noreferrer,width=980,height=820");
      }
      setStatusMessage("Google Drive connection started. Complete OAuth and folder selection in the opened tab.");
      schedulePolling(session.sessionId);
    } catch (error) {
      setStatusMessage(error instanceof Error ? error.message : "Unable to start Google Drive connection.");
    } finally {
      setStartingSession(false);
    }
  }

  async function selectProvider(provider: "supabase" | "google_drive") {
    if (!selectedProjectId) {
      return;
    }

    setSavingProvider(provider);
    startProviderSwitchProgress(provider);
    setStatusMessage(null);
    try {
      const response = await fetch("/api/local-runner/artifact-storage/switch-provider", {
        method: "POST",
        headers: {
          "content-type": "application/json",
          ...(await buildAuthHeaders()),
        },
        body: JSON.stringify({
          projectId: selectedProjectId,
          targetProvider: provider,
        }),
      });
      if (response.status === 401) {
        router.replace("/login");
        return;
      }
      const payload = (await response.json()) as ProviderSwitchResult | { error?: string };
      if (!response.ok) {
        if ("error" in payload && payload.error === "Authentication required.") {
          router.replace("/login");
          return;
        }
        throw new Error("error" in payload && payload.error ? payload.error : "Unable to switch artifact storage provider.");
      }

      const result = payload as ProviderSwitchResult;
      clearSwitchProgressTimers();
      setProviderSwitchState({
        targetProvider: provider,
        stage: result.status === "completed" ? "completed" : "failed",
        message:
          result.status === "completed"
            ? "Provider switch completed."
            : "Provider switch failed before the active provider could change.",
        totalArtifacts: result.totalArtifacts,
        syncedCount: result.syncedCount,
        skippedCount: result.skippedCount,
        failedCount: result.failedCount,
        currentArtifactLabel: result.failures[0]?.artifactRunId ?? null,
        failures: result.failures,
      });
      if (result.status === "completed") {
        setProjectList((current) =>
          current.map((project) =>
            project.id === selectedProjectId
              ? { ...project, artifactStoragePreference: provider }
              : project,
          ),
        );
      }
      if (result.status === "completed") {
        setStatusMessage(
          `Provider switch completed. Synced ${result.syncedCount}, skipped ${result.skippedCount}, total ${result.totalArtifacts}.`,
        );
      } else {
        setStatusMessage(
          `Provider switch stopped with ${result.failedCount} failures. Synced ${result.syncedCount}, skipped ${result.skippedCount}. ${result.failures[0]?.reason ?? ""}`.trim(),
        );
      }
      await loadGoogleDriveState();
    } catch (error) {
      clearSwitchProgressTimers();
      const message = error instanceof Error ? error.message : "Unable to update artifact storage provider.";
      const reconnectRequired = /reconnect/i.test(message);
      if (provider === "google_drive") {
        autoProviderSwitchKeyRef.current = null;
      }
      setProviderSwitchState((current) =>
        current
          ? {
              ...current,
              stage: reconnectRequired ? "reconnect_required" : "failed",
              message,
              failedCount: Math.max(current.failedCount, 1),
            }
          : null,
      );
      setStatusMessage(message);
    } finally {
      setSavingProvider(null);
    }
  }

  useEffect(() => {
    stopPolling();
    autoProviderSwitchKeyRef.current = null;
    setStatusMessage(null);
    void loadGoogleDriveState();
    void loadGoogleDriveSetup();
    return () => {
      stopPolling();
      clearSwitchProgressTimers();
    };
  }, [selectedProjectId]);

  useEffect(() => {
    function handleMessage(event: MessageEvent) {
      if (
        event.data &&
        typeof event.data === "object" &&
        event.data.type === "flowpilot-google-drive-connected" &&
        (!event.data.projectId || event.data.projectId === selectedProjectId)
      ) {
        void loadGoogleDriveState(googleDriveState?.session?.sessionId);
      }
    }

    window.addEventListener("message", handleMessage);
    return () => window.removeEventListener("message", handleMessage);
  }, [googleDriveState?.session?.sessionId, selectedProjectId]);

  const connection = googleDriveState?.connection ?? null;
  const session = googleDriveState?.session ?? null;
  const googleDriveAccounts = googleDriveSetupStatus?.accounts ?? [];
  const selectedGoogleDriveAccount =
    googleDriveAccounts.find((account) => account.accountId === selectedGoogleDriveAccountId) ?? null;
  const providerPreference = selectedProject?.artifactStoragePreference ?? "supabase";
  const googleDriveConfigured = Boolean(googleDriveSetupStatus?.artifactSync?.configured);
  const googleDriveConnected = Boolean(
    connection?.status === "connected" && connection.folderId?.trim(),
  );
  const googleDriveAccountReady = Boolean(selectedGoogleDriveAccount?.accountReady && selectedGoogleDriveAccount?.mcpWriteReady);
  const providerLabel = providerPreference === "google_drive" ? "Google Drive" : "Supabase Storage";
  const detailProviderLabel = detailProvider === "google_drive" ? "Google Drive" : "Supabase Storage";

  useEffect(() => {
    setDetailProvider(providerPreference);
  }, [providerPreference, selectedProjectId]);

  useEffect(() => {
    const preferredAccountId =
      connection?.accountId?.trim() ||
      session?.accountId?.trim() ||
      googleDriveSetupStatus?.mcp?.accountId?.trim() ||
      (googleDriveAccounts.length === 1 ? googleDriveAccounts[0]?.accountId?.trim() : "") ||
      "";
    if (!preferredAccountId) {
      return;
    }
    setSelectedGoogleDriveAccountId((current) => current || preferredAccountId);
  }, [connection?.accountId, googleDriveAccounts, googleDriveSetupStatus?.mcp?.accountId, session?.accountId]);

  return (
    <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            Shared Cloud Storage
          </p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight">Project artifact provider state</h2>
          <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
            Supabase is ready by default. Google Drive requires runner-local OAuth and one folder selected through Google Picker on this host.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Badge tone={providerPreference === "supabase" ? "success" : "neutral"}>supabase</Badge>
          <Badge tone={providerPreference === "google_drive" ? "success" : "neutral"}>google_drive</Badge>
        </div>
      </div>

      <div className="mt-6 grid gap-4 xl:grid-cols-[minmax(0,20rem)_1fr]">
        <label className="space-y-2">
          <span className="text-sm font-medium">Project</span>
          <select
            className="w-full rounded-2xl border border-border bg-card px-4 py-3 text-sm outline-none"
            value={selectedProjectId}
            onChange={(event) => setSelectedProjectId(event.target.value)}
          >
            {projectList.map((project) => (
              <option key={project.id} value={project.id}>
                {project.name}
              </option>
            ))}
          </select>
        </label>

        <div className="rounded-2xl border border-border bg-card/80 p-5">
          <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">
            Active provider for this project
          </p>
          <div className="mt-2 flex flex-wrap items-center gap-3">
            <p className="text-sm font-semibold">{providerLabel}</p>
            <Badge tone={providerPreference === "google_drive" ? "warning" : "success"}>
              {providerPreference}
            </Badge>
          </div>
          <p className="mt-2 text-sm text-muted-foreground">
            Pick the provider explicitly for each project. Supabase is ready immediately, while Google
            Drive needs Google Console setup plus one connected folder on this runner.
          </p>
        </div>
      </div>

      <div className="mt-6 rounded-2xl border border-border bg-card/80 p-5">
        <label className="space-y-2">
          <span className="text-sm font-medium">Provider detail</span>
          <select
            className="w-full rounded-2xl border border-border bg-background px-4 py-3 text-sm outline-none"
            value={detailProvider}
            onChange={(event) => setDetailProvider(event.target.value as StorageProviderOption)}
          >
            <option value="supabase">Supabase</option>
            <option value="google_drive">Google Drive</option>
          </select>
        </label>

        <div
          className={`mt-4 rounded-2xl border bg-card/70 p-5 ${
            detailProvider === providerPreference ? "border-emerald-500/60 shadow-[0_0_0_1px_rgba(16,185,129,0.2)]" : "border-border"
          }`}
        >
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="text-lg font-semibold">{detailProviderLabel}</p>
              <p className="mt-1 text-sm text-muted-foreground">
                {detailProvider === "supabase"
                  ? "Keep artifact sync on the shared Supabase bucket. This option is available by default."
                  : "Use a runner-local Google Drive folder after Google Console setup is saved for artifact sync."}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              {detailProvider === providerPreference ? (
                <Badge tone="success">active</Badge>
              ) : null}
              {detailProvider === "google_drive" ? (
                <>
                  <Badge tone={googleDriveConfigured ? "success" : "warning"}>
                    {googleDriveConfigured ? "setup ready" : "setup needed"}
                  </Badge>
                  <Badge
                    tone={
                      connection?.status === "connected"
                        ? "success"
                        : connection?.status === "failed" || connection?.status === "reconnect_required"
                          ? "danger"
                          : "warning"
                    }
                  >
                    {connection?.status || "disconnected"}
                  </Badge>
                </>
              ) : null}
            </div>
          </div>

          {detailProvider === "supabase" ? (
            <>
              <div className="mt-4 rounded-2xl border border-border bg-background px-4 py-3">
                <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">Why choose it</p>
                <p className="mt-2 text-sm text-muted-foreground">
                  Best when you want the default shared storage path without runner-local Google Drive setup.
                </p>
              </div>

              {providerPreference !== "supabase" ? (
                <div className="mt-4 flex flex-wrap items-center gap-3">
                  <Button
                    type="button"
                    disabled={!selectedProjectId || savingProvider !== null}
                    onClick={() => {
                      void selectProvider("supabase");
                    }}
                  >
                    {savingProvider === "supabase" ? "Saving..." : "Select Supabase"}
                  </Button>
                </div>
              ) : null}
            </>
          ) : (
            <>
              <div className="mt-4 rounded-2xl border border-border bg-background px-4 py-3">
                <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">Prerequisite</p>
                <p className="mt-2 text-sm text-muted-foreground">
                  {googleDriveConfigured
                    ? "Google Console values for artifact sync are already configured on this host."
                    : "Save the Google Console values first so this runner can start the Google Drive connection flow."}
                </p>
              </div>

	              <div className="mt-4 grid gap-3 sm:grid-cols-2">
	                <InfoTile label="Selected folder" value={connection?.folderName || "Not connected"} />
	                <InfoTile label="Folder id" value={connection?.folderId || "Not selected"} />
	                <InfoTile label="Google account" value={connection?.accountEmail || "Unknown"} />
	                <InfoTile label="Last validated" value={connection?.lastValidatedAt || "Never"} />
	                <InfoTile label="Connected at" value={connection?.connectedAt || "Never"} />
	                <InfoTile label="Last error" value={connection?.lastError || session?.lastError || "None"} />
	              </div>

	              <div className="mt-4 rounded-2xl border border-border bg-background px-4 py-3">
	                <label className="space-y-2">
	                  <span className="text-sm font-medium">Project Google account</span>
	                  <select
	                    className="w-full rounded-2xl border border-border bg-card px-4 py-3 text-sm outline-none"
	                    value={selectedGoogleDriveAccountId}
	                    onChange={(event) => setSelectedGoogleDriveAccountId(event.target.value)}
	                  >
	                    <option value="">Select a connected account</option>
	                    {googleDriveAccounts.map((account) => (
	                      <option key={account.accountId} value={account.accountId}>
	                        {account.accountEmail ? `${account.accountEmail} (${account.accountId})` : account.accountId}
	                      </option>
	                    ))}
	                  </select>
	                </label>
	                <p className="mt-2 text-xs text-muted-foreground">
	                  Pick the Google account for this project first, then choose the artifact folder that this project should use.
	                </p>
	                {selectedGoogleDriveAccount ? (
	                  <p className="mt-2 text-xs text-muted-foreground">
	                    Account status: {selectedGoogleDriveAccount.status}. Artifact write: {selectedGoogleDriveAccount.mcpWriteReady ? "ready" : "missing scope"}.
	                  </p>
	                ) : null}
	              </div>

	              {googleDriveConfigured && session?.connectUrl ? (
	                <div className="mt-4 rounded-2xl border border-dashed border-border bg-background/70 p-4">
	                  <p className="text-sm font-medium">Active connect link</p>
                  <a
                    className="mt-2 inline-flex items-center gap-2 text-sm text-primary underline-offset-4 hover:underline"
                    href={session.connectUrl}
                    rel="noreferrer"
                    target="_blank"
                  >
                    <Link2 className="size-4" />
                    Open connect flow
                  </a>
                  <p className="mt-2 text-xs text-muted-foreground">
                    Session expires at {session.expiresAt || "unknown"}.
                  </p>
                </div>
              ) : null}

              <div className="mt-4 flex flex-wrap items-center gap-3">
	                {!googleDriveConfigured ? (
	                  <a
                    className="inline-flex items-center justify-center rounded-md border border-input bg-background px-4 py-2 text-sm font-medium shadow-sm transition-colors hover:bg-accent hover:text-accent-foreground"
                    href="/settings/google-drive-setup"
                  >
                    Complete Google Console setup
                  </a>
	                ) : (
	                  <Button
	                    type="button"
	                    disabled={!selectedProjectId || startingSession || !selectedGoogleDriveAccountId || !googleDriveAccountReady}
	                    onClick={() => {
	                      void startGoogleDriveConnect();
	                    }}
	                  >
	                    <FolderOpen className="mr-2 size-4" />
	                    {startingSession
	                      ? "Starting..."
	                      : connection?.status === "connected"
	                        ? "Choose Google Drive folder"
	                        : "Choose Google Drive folder"}
	                  </Button>
	                )}
                {providerPreference !== "google_drive" ? (
                  <Button
                    type="button"
                    disabled={!selectedProjectId || savingProvider !== null || !googleDriveConfigured || !googleDriveConnected}
                    onClick={() => {
                      void selectProvider("google_drive");
                    }}
                  >
                    {savingProvider === "google_drive" ? "Saving..." : "Select Google Drive"}
                  </Button>
                ) : null}
                <Button
                  type="button"
                  variant="secondary"
                  disabled={!selectedProjectId || loadingState || loadingSetupState}
                  onClick={() => {
                    void loadGoogleDriveState(session?.sessionId);
                    void loadGoogleDriveSetup();
                  }}
                >
                  <RefreshCw className="mr-2 size-4" />
                  {loadingState || loadingSetupState ? "Refreshing..." : "Refresh status"}
                </Button>
              </div>
            </>
          )}

	          <p className="mt-4 text-sm text-muted-foreground">
	            {statusMessage ??
	              (detailProvider === "google_drive"
	                ? googleDriveConfigured
	                  ? "Select the Google account for this project, choose its artifact folder, then select Google Drive as the active provider."
	                  : "Finish Google Console setup first, then return here to connect a Google account and choose a project folder."
	                : "Supabase stays available immediately and does not require any extra host setup.")}
	          </p>
        </div>
      </div>

      {providerSwitchState ? (
        <div className="mt-6 rounded-2xl border border-primary/40 bg-card/90 p-5 shadow-sm">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">Provider switch migration</p>
              <p className="mt-2 text-sm font-medium">
                Target provider: {providerSwitchState.targetProvider === "google_drive" ? "Google Drive" : "Supabase"}
              </p>
              <p className="mt-2 text-sm text-muted-foreground">{providerSwitchState.message}</p>
              {providerSwitchState.currentArtifactLabel ? (
                <p className="mt-2 text-xs text-muted-foreground">
                  Current artifact: {providerSwitchState.currentArtifactLabel}
                </p>
              ) : null}
            </div>
            <Badge tone={providerSwitchStageTone(providerSwitchState.stage)}>
              {formatProviderSwitchStage(providerSwitchState.stage)}
            </Badge>
          </div>

          <div className="mt-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
            <InfoTile label="State" value={formatProviderSwitchStage(providerSwitchState.stage)} />
            <InfoTile label="Total artifacts" value={String(providerSwitchState.totalArtifacts)} />
            <InfoTile label="Synced" value={String(providerSwitchState.syncedCount)} />
            <InfoTile label="Skipped" value={String(providerSwitchState.skippedCount)} />
            <InfoTile label="Failed" value={String(providerSwitchState.failedCount)} />
          </div>

          <ol className="mt-4 grid gap-2">
            {providerSwitchStages.map((stage) => (
              <li
                key={stage}
                className={`flex items-center gap-2 text-sm font-medium ${
                  providerSwitchState.stage === stage
                    ? providerSwitchStageTextClass(stage)
                    : providerSwitchStageReached(providerSwitchState.stage, stage)
                      ? "text-emerald-500"
                  : "text-muted-foreground"
                }`}
              >
                {providerSwitchStageIcon(providerSwitchState.stage, stage)}
                {formatProviderSwitchStage(stage)}
              </li>
            ))}
          </ol>

          {providerSwitchState.failures.length > 0 ? (
            <div className="mt-4 rounded-2xl border border-border bg-background/70 p-4">
              <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">Failure details</p>
              <ul className="mt-3 space-y-2 text-sm text-muted-foreground">
                {providerSwitchState.failures.slice(0, 3).map((failure) => (
                  <li key={`${failure.artifactRunId}-${failure.reason}`}>
                    <span className="font-medium text-foreground">{failure.artifactRunId}</span>: {failure.reason}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

function InfoTile({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-2xl border border-border bg-background px-4 py-3">
      <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
      <p className="mt-2 break-words text-sm font-medium">{value}</p>
    </div>
  );
}

const providerSwitchStages: ProviderSwitchStage[] = [
  "validating_target_provider",
  "scanning_artifacts",
  "checking_existing_replicas",
  "syncing",
  "completed",
  "failed",
  "reconnect_required",
];

function formatProviderSwitchStage(stage: ProviderSwitchStage) {
  switch (stage) {
    case "validating_target_provider":
      return "validating target provider";
    case "scanning_artifacts":
      return "scanning artifacts";
    case "checking_existing_replicas":
      return "checking existing replicas";
    case "syncing":
      return "syncing";
    case "completed":
      return "completed";
    case "failed":
      return "failed";
    case "reconnect_required":
      return "reconnect required";
  }
}

function providerSwitchStageTone(stage: ProviderSwitchStage): "success" | "warning" | "danger" | "neutral" {
  switch (stage) {
    case "completed":
      return "success";
    case "failed":
    case "reconnect_required":
      return "danger";
    case "validating_target_provider":
    case "scanning_artifacts":
    case "checking_existing_replicas":
    case "syncing":
      return "warning";
  }
}

function providerSwitchStageTextClass(stage: ProviderSwitchStage) {
  switch (providerSwitchStageTone(stage)) {
    case "success":
      return "text-emerald-400";
    case "danger":
      return "text-danger";
    case "warning":
      return "text-amber-400";
    case "neutral":
      return "text-muted-foreground";
  }
}

function providerSwitchStageIcon(current: ProviderSwitchStage, candidate: ProviderSwitchStage) {
  if (candidate === current) {
    if (current === "completed") {
      return <Check className="size-4" aria-hidden="true" />;
    }
    if (current === "failed" || current === "reconnect_required") {
      return <span className="size-4" aria-hidden="true" />;
    }
    return <Loader2 className="size-4 animate-spin" aria-hidden="true" />;
  }
  if (providerSwitchStageReached(current, candidate)) {
    return <Check className="size-4" aria-hidden="true" />;
  }
  return <span className="size-4" aria-hidden="true" />;
}

function providerSwitchStageReached(current: ProviderSwitchStage, candidate: ProviderSwitchStage) {
  const currentIndex = providerSwitchStages.indexOf(current);
  const candidateIndex = providerSwitchStages.indexOf(candidate);
  if (candidateIndex === -1 || currentIndex === -1) {
    return false;
  }
  if (current === "failed" || current === "reconnect_required") {
    return candidateIndex < providerSwitchStages.indexOf("failed");
  }
  return candidateIndex <= currentIndex;
}
