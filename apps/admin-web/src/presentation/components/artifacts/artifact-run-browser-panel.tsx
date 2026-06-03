"use client";

import { useEffect, useMemo, useState } from "react";
import { ExternalLink, Search, UploadCloud } from "lucide-react";

import type { Project } from "@/domain/model/entity/project";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";
import type { ArtifactRun, ArtifactSyncStatus } from "@/domain/model/entity/workflow-engine";
import { Badge } from "@/presentation/components/ui/badge";
import { Button } from "@/presentation/components/ui/button";
import type { ReactNode } from "react";

interface ArtifactRunBrowserPanelProps {
  localArtifacts?: LocalRunnerArtifact[];
  artifactRuns?: ArtifactRun[];
  projects?: Project[];
  scopeLabel: string;
  showProjectFilter?: boolean;
  onArtifactsChanged?: () => Promise<void> | void;
}

type ArtifactBrowserItem = {
  id: string;
  kind: "local" | "remote";
  title: string;
  contentMarkdown: string;
  promptText: string;
  actualPromptText: string;
  projectId: string | null;
  projectName: string | null;
  workflowRunId: string;
  workflowRunStepId: string | null;
  artifactDefinitionKey: string;
  localPath: string;
  remotePath: string;
  remoteObjectId: string | null;
  storageProvider: "supabase" | "google_drive" | null;
  syncStatus: ArtifactSyncStatus;
  createdAt: string;
  updatedAt: string;
};

export function ArtifactRunBrowserPanel({
  localArtifacts = [],
  artifactRuns = [],
  projects = [],
  scopeLabel,
  showProjectFilter = false,
  onArtifactsChanged,
}: ArtifactRunBrowserPanelProps) {
  const [query, setQuery] = useState("");
  const [projectId, setProjectId] = useState("all");
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const [bulkSyncing, setBulkSyncing] = useState(false);
  const [subTab, setSubTab] = useState<"local" | "remote">("local");

  const projectById = useMemo(() => new Map(projects.map((project) => [project.id, project])), [projects]);
  const localArtifactById = useMemo(
    () => new Map(localArtifacts.map((artifact) => [artifact.artifactId, artifact])),
    [localArtifacts],
  );
  const localArtifactByRemotePath = useMemo(
    () =>
      new Map(
        localArtifacts
          .filter((artifact) => artifact.remotePath.trim() !== "")
          .map((artifact) => [artifact.remotePath.trim(), artifact]),
      ),
    [localArtifacts],
  );

  const remoteArtifacts = useMemo<ArtifactBrowserItem[]>(
    () =>
      artifactRuns
        .filter(shouldRenderArtifactRunAsRemote)
        .map((artifact) => {
          const matchedLocalArtifact =
            localArtifactById.get(artifact.id) ?? localArtifactByRemotePath.get(artifact.remotePath.trim());
          return {
            id: artifact.id,
            kind: "remote" as const,
            title: deriveArtifactDisplayTitle(
              matchedLocalArtifact?.contentMarkdown ?? "",
              artifact.title,
            ),
            contentMarkdown: matchedLocalArtifact?.contentMarkdown ?? "",
            promptText: matchedLocalArtifact?.promptText ?? "",
            actualPromptText: matchedLocalArtifact?.actualPromptText ?? "",
            projectId: artifact.projectId,
            projectName: artifact.projectId ? projectById.get(artifact.projectId)?.name ?? null : null,
            workflowRunId: artifact.workflowRunId,
            workflowRunStepId: artifact.workflowRunStepId,
            artifactDefinitionKey: formatArtifactDefinitionKey(artifact.artifactDefinitionKey),
            localPath: artifact.localPath,
            remotePath: artifact.remotePath,
            remoteObjectId: artifact.remoteObjectId,
            storageProvider: artifact.storageProvider,
            syncStatus: artifact.syncStatus,
            createdAt: artifact.createdAt,
            updatedAt: artifact.updatedAt,
          };
        }),
    [artifactRuns, localArtifactById, localArtifactByRemotePath, projectById],
  );

  const remoteArtifactKeys = useMemo(
    () => new Set(remoteArtifacts.map((artifact) => artifactRemoteKey(artifact))),
    [remoteArtifacts],
  );
  const remoteArtifactIds = useMemo(
    () => new Set(remoteArtifacts.map((artifact) => artifact.id)),
    [remoteArtifacts],
  );

  const syntheticRemoteArtifacts = useMemo<ArtifactBrowserItem[]>(
    () =>
      localArtifacts
        .filter((artifact) => shouldRenderLocalArtifactAsRemote(artifact))
        .filter((artifact) => !remoteArtifactKeys.has(localArtifactRemoteKey(artifact)))
        .map((artifact) => ({
          id: artifact.artifactId,
          kind: "remote" as const,
          title: deriveArtifactDisplayTitle(artifact.contentMarkdown, artifact.title),
          contentMarkdown: artifact.contentMarkdown,
          promptText: artifact.promptText ?? "",
          actualPromptText: artifact.actualPromptText ?? "",
          projectId: artifact.projectId ?? null,
          projectName: artifact.projectId ? projectById.get(artifact.projectId)?.name ?? null : null,
          workflowRunId: artifact.workflowRunId,
          workflowRunStepId: artifact.workflowStepKey || null,
          artifactDefinitionKey: artifact.sourceKind || artifact.workflowStepKey,
          localPath: artifact.localPath,
          remotePath: artifact.remotePath,
          remoteObjectId: artifact.remoteObjectId ?? null,
          storageProvider:
            artifact.storageProvider === "google_drive"
              ? "google_drive"
              : artifact.storageProvider === "supabase"
                ? "supabase"
                : null,
          syncStatus: normalizeLocalArtifactSyncStatus(artifact),
          createdAt: artifact.createdAt,
          updatedAt: artifact.updatedAt,
        })),
    [localArtifacts, projectById, remoteArtifactKeys],
  );

  const allRemoteArtifacts = useMemo(
    () => [...remoteArtifacts, ...syntheticRemoteArtifacts],
    [remoteArtifacts, syntheticRemoteArtifacts],
  );

  const localOnlyArtifacts = useMemo<ArtifactBrowserItem[]>(
    () =>
      localArtifacts
        .filter((artifact) => !hasRemoteArtifactCounterpart(artifact, remoteArtifactIds, remoteArtifactKeys))
        .filter((artifact) => !shouldRenderLocalArtifactAsRemote(artifact))
        .map((artifact) => ({
          id: artifact.artifactId,
          kind: "local",
          title: deriveArtifactDisplayTitle(artifact.contentMarkdown, artifact.title),
          contentMarkdown: artifact.contentMarkdown,
          promptText: artifact.promptText ?? "",
          actualPromptText: artifact.actualPromptText ?? "",
          projectId: artifact.projectId ?? null,
          projectName: artifact.projectId ? projectById.get(artifact.projectId)?.name ?? null : null,
          workflowRunId: artifact.workflowRunId,
          workflowRunStepId: artifact.workflowStepKey || null,
          artifactDefinitionKey: artifact.sourceKind || artifact.workflowStepKey,
          localPath: artifact.localPath,
          remotePath: artifact.remotePath,
          remoteObjectId: artifact.remoteObjectId ?? null,
          storageProvider: artifact.storageProvider === "google_drive" ? "google_drive" : artifact.storageProvider === "supabase" ? "supabase" : null,
          syncStatus: normalizeLocalArtifactSyncStatus(artifact),
          createdAt: artifact.createdAt,
          updatedAt: artifact.updatedAt,
        })),
    [localArtifacts, projectById, remoteArtifactIds, remoteArtifactKeys],
  );

  const projectOptions = useMemo(
    () =>
      uniqueValues([
        ...projects.map((project) => project.id),
        ...allRemoteArtifacts.map((artifact) => artifact.projectId ?? ""),
        ...localOnlyArtifacts.map((artifact) => artifact.projectId ?? ""),
      ]),
    [allRemoteArtifacts, localOnlyArtifacts, projects],
  );

  const filteredLocalArtifacts = localOnlyArtifacts.filter((artifact) => {
    const matchesQuery =
      query.trim() === "" || artifactMatchesQuery(artifact, query.toLowerCase());
    const matchesProject = projectId === "all" || (artifact.projectId ?? "") === projectId;
    return matchesQuery && matchesProject;
  });

  const filteredRemoteArtifacts = allRemoteArtifacts.filter((artifact) => {
    const matchesQuery =
      query.trim() === "" || artifactMatchesQuery(artifact, query.toLowerCase());
    const matchesProject = projectId === "all" || (artifact.projectId ?? "") === projectId;
    return matchesQuery && matchesProject;
  });

  const eligibleSyncArtifacts = useMemo(
    () =>
      uniqueValues([
        ...filteredLocalArtifacts.map((artifact) => artifact.id),
        ...filteredRemoteArtifacts
          .filter((artifact) => artifact.syncStatus === "local_only" || artifact.syncStatus === "failed")
          .map((artifact) => artifact.id),
      ]),
    [filteredLocalArtifacts, filteredRemoteArtifacts],
  );

  const syncingArtifactCount = useMemo(
    () =>
      uniqueValues([
        ...artifactRuns
          .filter((artifact) => artifact.syncStatus === "syncing")
          .map((artifact) => artifact.id),
        ...localArtifacts
          .filter((artifact) => normalizeLocalArtifactSyncStatus(artifact) === "syncing")
          .map((artifact) => artifact.artifactId),
      ]).length,
    [artifactRuns, localArtifacts],
  );

  useEffect(() => {
    if (!onArtifactsChanged || syncingArtifactCount === 0) {
      return;
    }

    const intervalId = window.setInterval(() => {
      void onArtifactsChanged();
    }, 5_000);

    return () => window.clearInterval(intervalId);
  }, [onArtifactsChanged, syncingArtifactCount]);

  async function syncArtifactById(artifactId: string) {
    try {
      const response = await fetch(`/api/local-runner/artifacts/${artifactId}/sync`, {
        method: "POST",
      });
      if (response.status === 401) {
        window.location.assign("/login");
        return { status: "auth-redirected" as const };
      }
      const payload = (await response.json()) as { error?: string; syncStatus?: string };
      if (!response.ok) {
        if (payload.error === "Authentication required.") {
          window.location.assign("/login");
          return { status: "auth-redirected" as const };
        }
        return {
          status: "failed" as const,
          error: payload.error || "Unable to sync artifact.",
        };
      }
      return payload.syncStatus === "synced"
        ? { status: "synced" as const }
        : {
            status: "failed" as const,
            error: payload.error || "Unable to sync artifact.",
          };
    } catch (error) {
      if (error instanceof Error && error.message === "Authentication required.") {
        window.location.assign("/login");
        return { status: "auth-redirected" as const };
      }
      return {
        status: "failed" as const,
        error: error instanceof Error ? error.message : "Unable to sync artifact.",
      };
    }
  }

  async function syncAllArtifacts() {
    if (eligibleSyncArtifacts.length === 0) {
      return;
    }
    setBulkSyncing(true);
    setStatusMessage(`Syncing ${eligibleSyncArtifacts.length} artifacts...`);
    try {
      let syncedCount = 0;
      const failures: string[] = [];
      for (const artifactId of eligibleSyncArtifacts) {
        const result = await syncArtifactById(artifactId);
        if (result.status === "auth-redirected") {
          return;
        }
        if (result.status === "synced") {
          syncedCount += 1;
        } else if (result.error) {
          failures.push(result.error);
        }
      }

      const summary =
        syncedCount === eligibleSyncArtifacts.length
          ? `Synced ${syncedCount} artifacts.`
          : `Synced ${syncedCount} of ${eligibleSyncArtifacts.length} artifacts.`;
      setStatusMessage(failures[0] ? `${summary} ${failures[0]}` : summary);
      await onArtifactsChanged?.();
    } catch (error) {
      setStatusMessage(error instanceof Error ? error.message : "Unable to sync artifacts.");
    } finally {
      setBulkSyncing(false);
    }
  }

  const totalArtifacts = filteredLocalArtifacts.length + filteredRemoteArtifacts.length;

  return (
    <section className="rounded-[1.6rem] border border-border bg-background/70 p-6">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.28em] text-muted-foreground">
            {scopeLabel}
          </p>
          <h2 className="mt-3 text-2xl font-semibold tracking-tight">Artifacts</h2>
          <p className="mt-2 max-w-3xl text-sm text-muted-foreground">
            Local artifacts are shown separately from remote Supabase-backed state so you can sync
            unsynced work without losing visibility into what is already shared.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <Badge tone="neutral">{totalArtifacts} artifacts</Badge>
          <Button
            type="button"
            disabled={bulkSyncing || syncingArtifactCount > 0 || eligibleSyncArtifacts.length === 0}
            onClick={() => {
              void syncAllArtifacts();
            }}
            variant="secondary"
          >
            <UploadCloud className="mr-2 size-4" />
            {bulkSyncing || syncingArtifactCount > 0
              ? "Syncing..."
              : `Sync artifacts (${eligibleSyncArtifacts.length})`}
          </Button>
        </div>
      </div>

      <div className="mt-6 grid gap-3 xl:grid-cols-3">
        <label className="space-y-2 xl:col-span-3">
          <span className="text-sm font-medium">Search</span>
          <div className="flex items-center gap-3 rounded-2xl border border-border bg-card px-4 py-3">
            <Search className="size-4 text-muted-foreground" />
            <input
              className="w-full bg-transparent text-sm outline-none"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search title, project, run id, path, or remote id"
            />
          </div>
        </label>

        {showProjectFilter ? (
          <label className="space-y-2">
            <span className="text-sm font-medium">Project</span>
            <select
              className="w-full rounded-2xl border border-border bg-card px-4 py-3 text-sm outline-none"
              value={projectId}
              onChange={(event) => setProjectId(event.target.value)}
            >
              <option value="all">All projects</option>
              {projectOptions.map((item) => (
                <option key={item} value={item}>
                  {projectById.get(item)?.name ?? item}
                </option>
              ))}
            </select>
          </label>
        ) : null}
      </div>

      {statusMessage ? <p className="mt-4 text-sm text-muted-foreground">{statusMessage}</p> : null}

      {/* Sub tabs for local vs remote */}
      <div className="mt-6 flex border border-border/30 bg-[#0f1119]/50 backdrop-blur-md rounded-2xl p-1 gap-1.5 w-fit shrink-0">
        <button
          type="button"
          className={`px-4 py-2.5 text-sm font-semibold tracking-wide transition-all duration-200 rounded-xl cursor-pointer ${
            subTab === "local"
              ? "text-primary bg-background shadow-md border border-border/20"
              : "text-muted-foreground hover:text-foreground hover:bg-muted/10"
          }`}
          onClick={() => setSubTab("local")}
        >
          local/NotSyned ({filteredLocalArtifacts.length})
        </button>
        <button
          type="button"
          className={`px-4 py-2.5 text-sm font-semibold tracking-wide transition-all duration-200 rounded-xl cursor-pointer ${
            subTab === "remote"
              ? "text-primary bg-background shadow-md border border-border/20"
              : "text-muted-foreground hover:text-foreground hover:bg-muted/10"
          }`}
          onClick={() => setSubTab("remote")}
        >
          Remote/Sync ({filteredRemoteArtifacts.length})
        </button>
      </div>

      {subTab === "local" ? (
        <ArtifactSection
          count={filteredLocalArtifacts.length}
          description="Valid local artifacts discovered on this runner. These are not yet represented in remote sync state."
          emptyMessage="No valid local-only artifacts match the current filters."
          title="Local"
        >
          {filteredLocalArtifacts.map((artifact) => (
            <ArtifactCard artifact={artifact} key={artifact.id} />
          ))}
        </ArtifactSection>
      ) : (
        <ArtifactSection
          count={filteredRemoteArtifacts.length}
          description="Artifacts that already have durable Supabase sync metadata."
          emptyMessage="No remote or synced artifacts match the current filters."
          title="Remote / Synced"
        >
          {filteredRemoteArtifacts.map((artifact) => (
            <ArtifactCard artifact={artifact} key={artifact.id} />
          ))}
        </ArtifactSection>
      )}
    </section>
  );
}

function ArtifactSection({
  count,
  description,
  emptyMessage,
  title,
  children,
}: {
  count: number;
  description: string;
  emptyMessage: string;
  title: string;
  children: ReactNode;
}) {
  return (
    <section className="mt-6 rounded-[1.6rem] border border-border/70 bg-card/60 p-5">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h3 className="text-xl font-semibold tracking-tight">{title}</h3>
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        </div>
        <Badge tone="neutral">{count}</Badge>
      </div>
      <div className="mt-5">
        {count === 0 ? (
          <p className="rounded-2xl border border-dashed border-border bg-background/60 p-5 text-sm text-muted-foreground">
            {emptyMessage}
          </p>
        ) : (
          <div className="flex flex-col gap-3">{children}</div>
        )}
      </div>
    </section>
  );
}

function ArtifactCard({
  artifact,
}: {
  artifact: ArtifactBrowserItem;
}) {
  const artifactFiles = buildArtifactFiles(artifact);

  return (
    <details className="group relative flex flex-col justify-between overflow-hidden rounded-[1.35rem] border border-border bg-background/80 p-4 shadow-sm backdrop-blur-md transition-all duration-300 hover:border-primary/20 hover:shadow-md [&[open]]:border-primary/30 [&[open]]:shadow-lg">
      <summary className="flex cursor-pointer list-none items-start justify-between gap-4 outline-none">
        <div className="min-w-0 flex-1">
          <p className="truncate text-base font-semibold group-hover:text-primary transition-colors">
            {artifact.title}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
            {artifact.projectName ?? artifact.projectId ?? "Unknown project"} / {artifact.workflowRunId}
          </p>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            {artifactFiles.map((file) => (
              <a
                className="inline-flex items-center gap-1 rounded-full border border-border px-2.5 py-1 text-xs font-medium hover:bg-muted transition-colors"
                href={file.href}
                key={file.label}
                rel="noreferrer"
                target="_blank"
              >
                <ExternalLink className="size-3" />
                {file.label}
              </a>
            ))}
          </div>
          {artifact.actualPromptText ? (
            <p className="mt-2 line-clamp-2 text-xs text-muted-foreground">
              Actual prompt: {truncateText(artifact.actualPromptText, 120)}
            </p>
          ) : null}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Badge tone={artifactStatusTone(artifact.syncStatus)}>{artifact.syncStatus}</Badge>
          <Badge tone="neutral">{artifact.storageProvider ?? (artifact.kind === "local" ? "local" : "unknown")}</Badge>
          <span className="ml-2 shrink-0 rounded border border-border bg-muted/30 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground/80 transition-all duration-300 group-hover:bg-primary group-hover:text-primary-foreground group-[[open]]:bg-primary group-[[open]]:text-primary-foreground">
            Details
          </span>
        </div>
      </summary>

      <div className="mt-4 border-t border-border/40 pt-4 space-y-3">
        <div>
          {artifact.actualPromptText ? (
            <div className="mb-3">
              <span className="text-xs font-semibold text-muted-foreground">Actual Prompt</span>
              <p className="mt-1 whitespace-pre-wrap break-words rounded-lg border border-border/30 bg-muted/40 p-2.5 text-xs">
                {artifact.actualPromptText}
              </p>
            </div>
          ) : null}

          {artifact.promptText && artifact.promptText !== artifact.actualPromptText ? (
            <div>
              <span className="text-xs font-semibold text-muted-foreground">Saved Prompt</span>
              <p className="mt-1 whitespace-pre-wrap break-words rounded-lg border border-border/30 bg-muted/20 p-2.5 text-xs">
                {artifact.promptText}
              </p>
            </div>
          ) : null}
        </div>

        <div>
          <span className="text-xs font-semibold text-muted-foreground">Path</span>
          <p className="mt-1 text-xs font-mono break-all rounded-lg border border-border/30 bg-muted/40 p-2.5">
            {artifact.remotePath || artifact.localPath || "No artifact path available."}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-4 text-xs">
          <div>
            <span className="font-semibold text-muted-foreground mr-1.5">Definition Key:</span>
            <Badge tone="neutral">{artifact.artifactDefinitionKey}</Badge>
          </div>
          <div>
            <span className="font-semibold text-muted-foreground mr-1.5">Step ID:</span>
            <Badge tone="neutral">{artifact.workflowRunStepId ?? "no step"}</Badge>
          </div>
          <div>
            <span className="font-semibold text-muted-foreground mr-1.5">Updated:</span>
            <Badge tone="neutral">
              {artifact.updatedAt.slice(0, 19).replace("T", " ")}
            </Badge>
          </div>
          {artifact.remoteObjectId ? (
            <div>
              <span className="font-semibold text-muted-foreground mr-1.5">Remote Object ID:</span>
              <Badge tone="neutral">{artifact.remoteObjectId}</Badge>
            </div>
          ) : null}
        </div>

        <div className="flex items-center justify-between border-t border-border/40 pt-3 mt-3">
          <div className="flex flex-wrap items-center gap-2">
            {artifactFiles.map((file) => (
              <a
                className="inline-flex items-center gap-2 rounded-full border border-border px-4 py-2 text-sm font-semibold hover:bg-muted transition-colors cursor-pointer"
                href={file.href}
                key={file.href}
                rel="noreferrer"
                target="_blank"
              >
                <ExternalLink className="size-4" />
                Open {file.label}
              </a>
            ))}
          </div>
        </div>
      </div>
    </details>
  );
}

function buildArtifactFiles(artifact: ArtifactBrowserItem) {
  const baseHref = buildArtifactOpenHref(artifact);
  const hasLocalSnapshotIdentity = !artifact.id.startsWith("remote:");
  const files = [
    {
      key: "content",
      label: preferredArtifactFileName(artifact),
      href: baseHref,
    },
  ];

  const canOpenActualPrompt =
    artifact.actualPromptText.trim() !== "" ||
    artifact.promptText.trim() !== "" ||
    (artifact.kind === "remote" && hasLocalSnapshotIdentity);
  if (canOpenActualPrompt) {
    files.push({
      key: "actual-prompt",
      label: "actual-prompt.md",
      href: appendArtifactFileQuery(baseHref, "actual-prompt"),
    });
  }

  const canOpenPrompt =
    artifact.promptText.trim() !== "" || (artifact.kind === "remote" && hasLocalSnapshotIdentity);
  if (canOpenPrompt) {
    files.push({
      key: "prompt",
      label: "prompt.md",
      href: appendArtifactFileQuery(baseHref, "prompt"),
    });
  }

  return files;
}

function buildArtifactOpenHref(artifact: ArtifactBrowserItem) {
  const params = new URLSearchParams();
  if (artifact.kind === "remote" && artifact.remotePath.trim()) {
    params.set("remotePath", artifact.remotePath.trim());
    if (artifact.storageProvider) {
      params.set("storageProvider", artifact.storageProvider);
    }
    if (artifact.remoteObjectId?.trim()) {
      params.set("remoteObjectId", artifact.remoteObjectId.trim());
    }
    if (artifact.projectId?.trim()) {
      params.set("projectId", artifact.projectId.trim());
    }
  }

  const query = params.toString();
  return query
    ? `/api/local-runner/artifacts/${artifact.id}/open?${query}`
    : `/api/local-runner/artifacts/${artifact.id}/open`;
}

function appendArtifactFileQuery(baseHref: string, file: string) {
  const separator = baseHref.includes("?") ? "&" : "?";
  return `${baseHref}${separator}file=${encodeURIComponent(file)}`;
}

function preferredArtifactFileName(artifact: ArtifactBrowserItem) {
  const trimmedTitle = artifact.title.trim();
  if (/response\.md$/i.test(trimmedTitle)) {
    return trimmedTitle;
  }

  return "Response.md";
}

function artifactMatchesQuery(artifact: ArtifactBrowserItem, query: string) {
  return [
    artifact.title,
    artifact.contentMarkdown,
    artifact.promptText,
    artifact.actualPromptText,
    artifact.projectName ?? "",
    artifact.projectId ?? "",
    artifact.localPath,
    artifact.remotePath,
    artifact.remoteObjectId ?? "",
    artifact.workflowRunId,
    artifact.workflowRunStepId ?? "",
    artifact.artifactDefinitionKey,
  ]
    .join(" ")
    .toLowerCase()
    .includes(query);
}

function formatArtifactDefinitionKey(artifactDefinitionKey: string | null) {
  const normalized = artifactDefinitionKey?.trim();
  return normalized ? normalized : "unbound";
}

function shouldRenderArtifactRunAsRemote(artifact: ArtifactRun) {
  const remotePath = artifact.remotePath.trim();
  const storageProvider = artifact.storageProvider?.trim() ?? "";
  return remotePath !== "" && storageProvider !== "";
}

function hasRemoteArtifactCounterpart(
  artifact: LocalRunnerArtifact,
  remoteArtifactIds: Set<string>,
  remoteArtifactKeys: Set<string>,
) {
  return (
    remoteArtifactIds.has(artifact.artifactId) ||
    (artifact.remotePath.trim() !== "" && remoteArtifactKeys.has(localArtifactRemoteKey(artifact)))
  );
}

function shouldRenderLocalArtifactAsRemote(artifact: LocalRunnerArtifact) {
  return normalizeLocalArtifactSyncStatus(artifact) !== "local_only";
}

function normalizeLocalArtifactSyncStatus(artifact: LocalRunnerArtifact): ArtifactSyncStatus {
  const normalizedStatus = artifact.syncStatus?.trim().toLowerCase();
  if (normalizedStatus === "synced") {
    return "synced";
  }
  if (normalizedStatus === "syncing") {
    return "syncing";
  }
  if (normalizedStatus === "failed" && hasLocalArtifactRemoteMetadata(artifact)) {
    return "failed";
  }
  return "local_only";
}

function hasLocalArtifactRemoteMetadata(artifact: LocalRunnerArtifact) {
  const storageProvider = artifact.storageProvider?.trim();
  const remotePath = artifact.remotePath.trim();
  return storageProvider === "supabase" || storageProvider === "google_drive"
    ? remotePath !== ""
    : false;
}

function localArtifactRemoteKey(artifact: LocalRunnerArtifact) {
  return artifact.remotePath.trim() || artifact.artifactId;
}

function artifactRemoteKey(artifact: Pick<ArtifactBrowserItem, "id" | "remotePath">) {
  return artifact.remotePath.trim() || artifact.id;
}

function artifactStatusTone(status: ArtifactSyncStatus) {
  switch (status) {
    case "synced":
      return "success";
    case "failed":
      return "danger";
    case "syncing":
      return "warning";
    default:
      return "neutral";
  }
}

function uniqueValues(values: string[]) {
  return Array.from(new Set(values.filter((value) => value.trim() !== "")));
}

function deriveArtifactDisplayTitle(contentMarkdown: string, fallbackTitle: string) {
  const normalized = contentMarkdown.replace(/\s+/g, " ").trim();
  if (!normalized) {
    return fallbackTitle;
  }

  return normalized.length > 50 ? `${normalized.slice(0, 50).trimEnd()}...` : normalized;
}

function truncateText(value: string, maxLength: number) {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= maxLength) {
    return normalized;
  }

  return `${normalized.slice(0, maxLength).trimEnd()}...`;
}
