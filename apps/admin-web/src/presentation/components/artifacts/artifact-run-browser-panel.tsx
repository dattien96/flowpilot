"use client";

import { useEffect, useMemo, useState } from "react";
import { ExternalLink, Search, UploadCloud } from "lucide-react";

import type { Project } from "@/domain/model/entity/project";
import type { LocalRunnerArtifact } from "@/domain/model/entity/local-runner";
import type { ArtifactRun, ArtifactSyncStatus } from "@/domain/model/entity/workflow-engine";
import { loadWorkflowRunArtifactPromptFiles } from "@/lib/workflow-run-artifact-open";
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
  loadRemoteArtifactContent?: (artifact: ArtifactRun) => Promise<string | null>;
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

type RemotePromptOverride = {
  actualPromptText: string;
  promptText: string;
};

export function ArtifactRunBrowserPanel({
  localArtifacts = [],
  artifactRuns = [],
  projects = [],
  scopeLabel,
  showProjectFilter = false,
  onArtifactsChanged,
  loadRemoteArtifactContent,
}: ArtifactRunBrowserPanelProps) {
  const [query, setQuery] = useState("");
  const [projectId, setProjectId] = useState("all");
  const [statusMessage, setStatusMessage] = useState<string | null>(null);
  const [bulkSyncing, setBulkSyncing] = useState(false);
  const [subTab, setSubTab] = useState<"local" | "remote">("local");
  const [remoteTitleOverrides, setRemoteTitleOverrides] = useState<Map<string, string>>(
    () => new Map(),
  );
  const [remotePromptOverrides, setRemotePromptOverrides] = useState<
    Map<string, RemotePromptOverride>
  >(() => new Map());

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
          const promptOverride = remotePromptOverrides.get(artifact.id);
          const fallbackTitle = deriveArtifactDisplayTitle(
            matchedLocalArtifact?.contentMarkdown ?? "",
            artifact.title,
          );
          return {
            id: artifact.id,
            kind: "remote" as const,
            title: remoteTitleOverrides.get(artifact.id) ?? fallbackTitle,
            contentMarkdown: matchedLocalArtifact?.contentMarkdown ?? "",
            promptText: matchedLocalArtifact?.promptText ?? promptOverride?.promptText ?? "",
            actualPromptText:
              matchedLocalArtifact?.actualPromptText ?? promptOverride?.actualPromptText ?? "",
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
    [
      artifactRuns,
      localArtifactById,
      localArtifactByRemotePath,
      projectById,
      remotePromptOverrides,
      remoteTitleOverrides,
    ],
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

  useEffect(() => {
    if (!loadRemoteArtifactContent) {
      setRemoteTitleOverrides(new Map());
      return;
    }

    const candidates = artifactRuns
      .filter(shouldRenderArtifactRunAsRemote)
      .filter((artifact) => {
        const matchedLocalArtifact =
          localArtifactById.get(artifact.id) ?? localArtifactByRemotePath.get(artifact.remotePath.trim());
        const hasLocalContent =
          (matchedLocalArtifact?.contentMarkdown ?? "").replace(/\s+/g, " ").trim() !== "";
        return !hasLocalContent && isGenericArtifactTitle(artifact.title);
      });

    if (candidates.length === 0) {
      setRemoteTitleOverrides(new Map());
      return;
    }

    let cancelled = false;

    void Promise.all(
      candidates.map(async (artifact) => {
        const content = await loadRemoteArtifactContent(artifact);
        const summary = content ? summarizeArtifactContent(content) : null;
        return summary ? ([artifact.id, summary] as const) : null;
      }),
    ).then((entries) => {
      if (cancelled) {
        return;
      }

      const next = new Map<string, string>();
      for (const entry of entries) {
        if (!entry) {
          continue;
        }
        next.set(entry[0], entry[1]);
      }
      setRemoteTitleOverrides(next);
    });

    return () => {
      cancelled = true;
    };
  }, [artifactRuns, loadRemoteArtifactContent, localArtifactById, localArtifactByRemotePath]);

  useEffect(() => {
    if (subTab !== "remote") {
      setRemotePromptOverrides(new Map());
      return;
    }

    const candidates = artifactRuns
      .filter(shouldRenderArtifactRunAsRemote)
      .filter((artifact) => {
        const matchedLocalArtifact =
          localArtifactById.get(artifact.id) ?? localArtifactByRemotePath.get(artifact.remotePath.trim());
        const hasLocalPrompt =
          (matchedLocalArtifact?.actualPromptText ?? "").trim() !== "" ||
          (matchedLocalArtifact?.promptText ?? "").trim() !== "";
        return !hasLocalPrompt && artifact.remotePath.trim() !== "";
      });

    if (candidates.length === 0) {
      setRemotePromptOverrides(new Map());
      return;
    }

    let cancelled = false;

    void Promise.all(
      candidates.map(async (artifact) => {
        const promptFiles = await loadWorkflowRunArtifactPromptFiles(artifact);
        if (
          !promptFiles.actualPromptText?.trim() &&
          !promptFiles.promptText?.trim()
        ) {
          return null;
        }

        return [
          artifact.id,
          {
            actualPromptText: promptFiles.actualPromptText ?? "",
            promptText: promptFiles.promptText ?? "",
          } satisfies RemotePromptOverride,
        ] as const;
      }),
    ).then((entries) => {
      if (cancelled) {
        return;
      }

      const next = new Map<string, RemotePromptOverride>();
      for (const entry of entries) {
        if (!entry) {
          continue;
        }

        next.set(entry[0], entry[1]);
      }
      setRemotePromptOverrides(next);
    });

    return () => {
      cancelled = true;
    };
  }, [artifactRuns, localArtifactById, localArtifactByRemotePath, subTab]);

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
          {renderGroupedArtifacts(filteredLocalArtifacts)}
        </ArtifactSection>
      ) : (
        <ArtifactSection
          count={filteredRemoteArtifacts.length}
          description="Artifacts that already have durable Supabase sync metadata."
          emptyMessage="No remote or synced artifacts match the current filters."
          title="Remote / Synced"
        >
          {renderGroupedArtifacts(filteredRemoteArtifacts)}
        </ArtifactSection>
      )}
    </section>
  );

  function renderGroupedArtifacts(items: ArtifactBrowserItem[]) {
    const grouped = groupArtifacts(items);
    const showProjectLevel = showProjectFilter || grouped.length > 1;

    return (
      <div className="flex flex-col gap-6">
        {grouped.map((projectGroup) => {
          const projectContent = (
            <div className="flex flex-col gap-4" key={projectGroup.projectId}>
              {projectGroup.workflowRuns.map((runGroup) => (
                <WorkflowRunGroup
                  key={runGroup.workflowRunId}
                  runId={runGroup.workflowRunId}
                  projectName={runGroup.projectName}
                  artifacts={runGroup.artifacts}
                />
              ))}
            </div>
          );

          if (showProjectLevel) {
            return (
              <div key={projectGroup.projectId} className="space-y-4">
                <div className="flex items-center gap-2 border-b border-border/40 pb-2">
                  <h4 className="text-sm font-semibold uppercase tracking-wider text-primary">
                    {projectGroup.projectName}
                  </h4>
                  <Badge tone="neutral">
                    {projectGroup.workflowRuns.reduce((acc, r) => acc + r.artifacts.length, 0)}
                  </Badge>
                </div>
                {projectContent}
              </div>
            );
          }

          return projectContent;
        })}
      </div>
    );
  }
}

interface GroupedWorkflowRun {
  workflowRunId: string;
  projectName: string | null;
  artifacts: ArtifactBrowserItem[];
}

interface GroupedProject {
  projectId: string;
  projectName: string;
  workflowRuns: GroupedWorkflowRun[];
}

function groupArtifacts(artifacts: ArtifactBrowserItem[]): GroupedProject[] {
  const projectMap = new Map<string, Map<string, ArtifactBrowserItem[]>>();
  const projectNameMap = new Map<string, string>();

  for (const artifact of artifacts) {
    const pId = artifact.projectId || "unknown";
    const pName = artifact.projectName || "Unknown Project";
    projectNameMap.set(pId, pName);

    if (!projectMap.has(pId)) {
      projectMap.set(pId, new Map());
    }

    const runsMap = projectMap.get(pId)!;
    const runId = artifact.workflowRunId || "unknown";

    if (!runsMap.has(runId)) {
      runsMap.set(runId, []);
    }
    runsMap.get(runId)!.push(artifact);
  }

  const result: GroupedProject[] = [];
  projectMap.forEach((runsMap, pId) => {
    const pName = projectNameMap.get(pId) || "Unknown Project";
    const workflowRuns: GroupedWorkflowRun[] = [];

    runsMap.forEach((runArtifacts, runId) => {
      const sortedArtifacts = [...runArtifacts].sort(
        (a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
      );
      workflowRuns.push({
        workflowRunId: runId,
        projectName: pName,
        artifacts: sortedArtifacts,
      });
    });

    workflowRuns.sort((a, b) => {
      const aLatest = a.artifacts[0]?.createdAt ? new Date(a.artifacts[0].createdAt).getTime() : 0;
      const bLatest = b.artifacts[0]?.createdAt ? new Date(b.artifacts[0].createdAt).getTime() : 0;
      return bLatest - aLatest;
    });

    result.push({
      projectId: pId,
      projectName: pName,
      workflowRuns,
    });
  });

  return result.sort((a, b) => a.projectName.localeCompare(b.projectName));
}

function WorkflowRunGroup({
  runId,
  projectName,
  artifacts,
}: {
  runId: string;
  projectName: string | null;
  artifacts: ArtifactBrowserItem[];
}) {
  const [copied, setCopied] = useState(false);

  const status = useMemo(() => {
    const statuses = artifacts.map((a) => a.syncStatus);
    if (statuses.includes("syncing")) return "syncing";
    if (statuses.includes("failed")) return "failed";
    if (statuses.every((s) => s === "synced")) return "synced";
    return "local_only";
  }, [artifacts]);

  const handleCopy = (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    void navigator.clipboard.writeText(runId).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <details className="group/run relative flex flex-col justify-between overflow-hidden rounded-[1.6rem] border border-border bg-[#0f1119]/35 shadow-sm backdrop-blur-md transition-all duration-300 hover:border-primary/20 hover:shadow-md [&[open]]:border-primary/30 [&[open]]:shadow-lg">
      <summary className="flex cursor-pointer list-none items-center justify-between gap-4 p-5 outline-none hover:bg-muted/10 transition-colors">
        <div className="flex min-w-0 flex-1 items-start gap-3">
          <div className="mt-1 flex shrink-0 items-center justify-center text-muted-foreground group-open/run:rotate-90 transition-transform duration-200">
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="16"
              height="16"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              className="size-4"
            >
              <path d="m9 18 6-6-6-6" />
            </svg>
          </div>

          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
              <span className="font-mono text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                Workflow Run
              </span>
              <div className="flex items-center gap-1.5 font-mono text-sm font-medium text-foreground">
                <span className="truncate max-w-[180px] sm:max-w-none">{runId}</span>
                <button
                  onClick={handleCopy}
                  type="button"
                  className="rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground transition-colors cursor-pointer"
                  title="Copy Run ID"
                >
                  {copied ? (
                    <svg
                      xmlns="http://www.w3.org/2000/svg"
                      width="12"
                      height="12"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2.5"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      className="text-success"
                    >
                      <path d="M20 6 9 17l-5-5" />
                    </svg>
                  ) : (
                    <svg
                      xmlns="http://www.w3.org/2000/svg"
                      width="12"
                      height="12"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2"
                      strokeLinecap="round"
                      strokeLinejoin="round"
                    >
                      <rect width="14" height="14" x="8" y="8" rx="2" ry="2" />
                      <path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />
                    </svg>
                  )}
                </button>
              </div>
            </div>
          </div>
        </div>

        <div className="flex shrink-0 items-center gap-2">
          <Badge tone={artifactStatusTone(status)}>{status}</Badge>
          <Badge tone="neutral">
            {artifacts.length} {artifacts.length === 1 ? "artifact" : "artifacts"}
          </Badge>
          <a
            href={`/workflow-runs/${runId}`}
            onClick={(e) => {
              e.stopPropagation();
            }}
            className="flex items-center gap-1.5 rounded-xl border border-border/80 px-3.5 py-1.5 text-xs font-semibold text-muted-foreground bg-muted/20 hover:bg-primary hover:text-primary-foreground hover:border-transparent transition-all duration-300 cursor-pointer"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.5"
              strokeLinecap="round"
              strokeLinejoin="round"
              className="size-3.5"
            >
              <path d="M15 3h6v6" />
              <path d="M10 14 21 3" />
              <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
            </svg>
            Go to Run
          </a>
        </div>
      </summary>

      <div className="border-t border-border/40 bg-card/25 p-5 space-y-4">
        <div className="flex flex-col gap-4">
          {artifacts.map((artifact) => (
            <ArtifactCard artifact={artifact} key={artifact.id} isChild />
          ))}
        </div>
      </div>
    </details>
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
  isChild = false,
}: {
  artifact: ArtifactBrowserItem;
  isChild?: boolean;
}) {
  const artifactFiles = buildArtifactFiles(artifact);

  return (
    <details className={`group relative flex flex-col justify-between overflow-hidden rounded-[1.35rem] border shadow-sm backdrop-blur-md transition-all duration-300 ${
      isChild
        ? "border-border/60 bg-background/40 hover:border-primary/20 hover:shadow-sm [&[open]]:border-primary/20 [&[open]]:shadow-md p-3.5"
        : "border-border bg-background/80 hover:border-primary/20 hover:shadow-md [&[open]]:border-primary/30 [&[open]]:shadow-lg p-4"
    }`}>
      <summary className="flex cursor-pointer list-none items-start justify-between gap-4 outline-none">
        <div className="min-w-0 flex-1">
          <p className="truncate text-base font-semibold group-hover:text-primary transition-colors">
            {artifact.title}
          </p>
          {!isChild ? (
            <p className="mt-1 text-xs text-muted-foreground">
              {artifact.projectName ?? artifact.projectId ?? "Unknown project"} / {artifact.workflowRunId}
            </p>
          ) : null}
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

function summarizeArtifactContent(contentMarkdown: string) {
  const normalized = contentMarkdown.replace(/\s+/g, " ").trim();
  if (!normalized) {
    return null;
  }

  return normalized.length > 50
    ? `${normalized.slice(0, 50).trimEnd()}...`
    : normalized;
}

function isGenericArtifactTitle(title: string) {
  const normalized = title.trim().toLowerCase();
  return normalized === "response.md" || normalized === "artifact.md";
}

function deriveArtifactDisplayTitle(contentMarkdown: string, fallbackTitle: string) {
  const summary = summarizeArtifactContent(contentMarkdown);
  if (!summary) {
    return fallbackTitle;
  }

  return summary;
}

function truncateText(value: string, maxLength: number) {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= maxLength) {
    return normalized;
  }

  return `${normalized.slice(0, maxLength).trimEnd()}...`;
}
