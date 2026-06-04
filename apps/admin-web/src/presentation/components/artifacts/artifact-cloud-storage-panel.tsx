"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { ExternalLink, FolderOpen, Link2, RefreshCw } from "lucide-react";
import { useRouter } from "next/navigation";

import { getBrowserSupabaseClient } from "@/data/supabase/client";
import type { Project } from "@/domain/model/entity/project";
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

export function ArtifactCloudStoragePanel({ projects }: ArtifactCloudStoragePanelProps) {
  const router = useRouter();
  const [selectedProjectId, setSelectedProjectId] = useState(projects[0]?.id ?? "");
  const [googleDriveState, setGoogleDriveState] = useState<GoogleDriveConnectionPayload | null>(null);
  const [loadingState, setLoadingState] = useState(false);
  const [startingSession, setStartingSession] = useState(false);
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const pollingTimerRef = useRef<number | null>(null);

  const selectedProject = useMemo(
    () => projects.find((project) => project.id === selectedProjectId) ?? null,
    [projects, selectedProjectId],
  );

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

  useEffect(() => {
    stopPolling();
    setStatusMessage(null);
    void loadGoogleDriveState();
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
  const providerLabel = providerPreference === "google_drive" ? "Google Drive" : "Supabase Storage";

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
            {projects.map((project) => (
              <option key={project.id} value={project.id}>
                {project.name}
              </option>
            ))}
          </select>
        </label>

        <div className="rounded-2xl border border-border bg-card/80 p-5">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <p className="text-lg font-semibold">Google Drive folder connection</p>
              <p className="mt-1 text-sm text-muted-foreground">
                Status is local to this runner host. Shared folder metadata is mirrored best-effort after connection.
              </p>
            </div>
            <Badge tone={connection?.status === "connected" ? "success" : connection?.status === "failed" ? "danger" : "warning"}>
              {connection?.status || "disconnected"}
            </Badge>
          </div>

          <div className="mt-4 rounded-2xl border border-border bg-background px-4 py-3">
            <p className="text-xs uppercase tracking-[0.24em] text-muted-foreground">Active provider for this project</p>
            <div className="mt-2 flex flex-wrap items-center gap-3">
              <p className="text-sm font-semibold">{providerLabel}</p>
              <Badge tone={providerPreference === "google_drive" ? "warning" : "success"}>
                {providerPreference}
              </Badge>
            </div>
            <p className="mt-2 text-sm text-muted-foreground">
              {providerPreference === "google_drive"
                ? "This project will sync artifacts to Google Drive after this runner is connected to a folder."
                : "This project will sync artifacts to the shared Supabase bucket. Google Drive details below are optional until you switch providers."}
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

          {session?.connectUrl ? (
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
            <Button
              type="button"
              disabled={!selectedProjectId || startingSession}
              onClick={() => {
                void startGoogleDriveConnect();
              }}
            >
              <FolderOpen className="mr-2 size-4" />
              {startingSession ? "Starting..." : connection?.status === "connected" ? "Reconnect Google Drive" : "Connect Google Drive"}
            </Button>
            <Button
              type="button"
              variant="secondary"
              disabled={!selectedProjectId || loadingState}
              onClick={() => {
                void loadGoogleDriveState(session?.sessionId);
              }}
            >
              <RefreshCw className="mr-2 size-4" />
              {loadingState ? "Refreshing..." : "Refresh status"}
            </Button>
            {selectedProject ? (
              <a
                className="inline-flex items-center gap-2 rounded-full border border-border px-4 py-2 text-sm font-semibold hover:bg-muted"
                href={`/projects/${selectedProject.id}/settings`}
                target="_blank"
                rel="noreferrer"
              >
                <ExternalLink className="size-4" />
                Project settings
              </a>
            ) : null}
          </div>

          <p className="mt-4 text-sm text-muted-foreground">
            {statusMessage ??
              "Use Connect Google Drive to launch OAuth on this runner host, then choose one folder with Google Picker."}
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
