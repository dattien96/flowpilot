"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { FolderOpen, Link2, RefreshCw } from "lucide-react";
import { useRouter } from "next/navigation";

import { createGatewayBundle } from "@/data/repository/browser-factory";
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
    lastError?: string;
  } | null;
};

interface ArtifactCloudStoragePanelProps {
  projects: Project[];
}

type StorageProviderOption = "supabase" | "google_drive";

export function ArtifactCloudStoragePanel({ projects }: ArtifactCloudStoragePanelProps) {
  const router = useRouter();
  const gatewayBundle = useRef(createGatewayBundle());
  const [projectList, setProjectList] = useState(projects);
  const [selectedProjectId, setSelectedProjectId] = useState(projects[0]?.id ?? "");
  const [detailProvider, setDetailProvider] = useState<StorageProviderOption>("supabase");
  const [googleDriveState, setGoogleDriveState] = useState<GoogleDriveConnectionPayload | null>(null);
  const [googleDriveSetupStatus, setGoogleDriveSetupStatus] = useState<GoogleDriveRuntimeStatus | null>(null);
  const [loadingState, setLoadingState] = useState(false);
  const [loadingSetupState, setLoadingSetupState] = useState(false);
  const [startingSession, setStartingSession] = useState(false);
  const [savingProvider, setSavingProvider] = useState<"supabase" | "google_drive" | null>(null);
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const pollingTimerRef = useRef<number | null>(null);

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
    return token ? { Authorization: `Bearer ${token}` } : {};
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
      setGoogleDriveState(payload as GoogleDriveConnectionPayload);
      const session = (payload as GoogleDriveConnectionPayload).session;
      if (session && (session.status === "pending" || session.status === "awaiting_oauth" || session.status === "awaiting_folder_selection")) {
        schedulePolling(session.sessionId);
      } else {
        stopPolling();
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

  function schedulePolling(sessionId: string) {
    stopPolling();
    pollingTimerRef.current = window.setTimeout(() => {
      void loadGoogleDriveState(sessionId);
    }, 2000);
  }

  async function startGoogleDriveConnect() {
    if (!selectedProjectId) {
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

    const googleDriveReady =
      provider === "google_drive" &&
      googleDriveSetupStatus?.artifactSync?.configured &&
      connection?.status === "connected" &&
      Boolean(connection.folderId?.trim());

    if (provider === "google_drive" && !googleDriveReady) {
      setStatusMessage(
        googleDriveSetupStatus?.artifactSync?.configured
          ? "Connect a Google Drive folder on this runner before selecting Google Drive."
          : "Complete Google Console setup before selecting Google Drive.",
      );
      return;
    }

    setSavingProvider(provider);
    setStatusMessage(null);
    try {
      const updatedProject = await gatewayBundle.current.projectGateway.updateProject(selectedProjectId, {
        artifactStoragePreference: provider,
      });
      setProjectList((current) =>
        current.map((project) => (project.id === updatedProject.id ? updatedProject : project)),
      );
      setStatusMessage(
        provider === "google_drive"
          ? "Google Drive is now the selected artifact storage provider for this project."
          : "Supabase Storage is now the selected artifact storage provider for this project.",
      );
    } catch (error) {
      setStatusMessage(error instanceof Error ? error.message : "Unable to update artifact storage provider.");
    } finally {
      setSavingProvider(null);
    }
  }

  useEffect(() => {
    stopPolling();
    setStatusMessage(null);
    void loadGoogleDriveState();
    void loadGoogleDriveSetup();
    return () => {
      stopPolling();
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
  const providerPreference = selectedProject?.artifactStoragePreference ?? "supabase";
  const googleDriveConfigured = Boolean(googleDriveSetupStatus?.artifactSync?.configured);
  const googleDriveConnected = Boolean(
    connection?.status === "connected" && connection.folderId?.trim(),
  );
  const providerLabel = providerPreference === "google_drive" ? "Google Drive" : "Supabase Storage";
  const detailProviderLabel = detailProvider === "google_drive" ? "Google Drive" : "Supabase Storage";

  useEffect(() => {
    setDetailProvider(providerPreference);
  }, [providerPreference, selectedProjectId]);

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
                  <Badge tone={connection?.status === "connected" ? "success" : connection?.status === "failed" ? "danger" : "warning"}>
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
                    disabled={!selectedProjectId || startingSession}
                    onClick={() => {
                      void startGoogleDriveConnect();
                    }}
                  >
                    <FolderOpen className="mr-2 size-4" />
                    {startingSession
                      ? "Starting..."
                      : connection?.status === "connected"
                        ? "Reconnect Google Drive"
                        : "Connect Google Drive"}
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
                  ? "Connect Google Drive on this runner, choose one folder, then select Google Drive as the active provider."
                  : "Finish Google Console setup first, then return here to connect a Google Drive folder and select it."
                : "Supabase stays available immediately and does not require any extra host setup.")}
          </p>
        </div>
      </div>
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
