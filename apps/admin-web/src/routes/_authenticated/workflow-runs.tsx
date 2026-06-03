import { useEffect, useMemo, useRef, useState } from "react";
import {
  createFileRoute,
  Link,
  Outlet,
  useLocation,
} from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createSupabaseBrowserClient } from "@/data/datasource/supabase/client";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { ListWorkflowRunsUseCase } from "@/domain/usecase/workflow-engine/list-workflow-runs-usecase";
import { ListWorkflowsUseCase } from "@/domain/usecase/workflow-engine/list-workflows-usecase";
import type { Project } from "@/domain/model/entity/project";
import type {
  Workflow,
  WorkflowRun,
  WorkflowRunSession,
} from "@/domain/model/entity/workflow-engine";
import { getMissingWorkflowRunSessionIds } from "@/features/workflow-engine/workflow-run-session-liveness";
import { markWorkflowRunSessionsCompleted } from "@/features/workflow-engine/workflow-run-session-timeout";
import { loadWorkflowRunTitleMap } from "@/lib/workflow-run-title";

export const Route = createFileRoute("/_authenticated/workflow-runs")({
  component: WorkflowRunHistoryPage,
});

type WorkflowRunSessionRow = {
  id: string;
  workflow_run_id: string;
  provider: string;
  model: string;
  transport_type: string;
  provider_session_id: string | null;
  process_key: string | null;
  process_pid: number | null;
  status: string;
  started_at: string;
  completed_at: string | null;
};

function mapWorkflowRunSessionRow(row: WorkflowRunSessionRow): WorkflowRunSession {
  return {
    id: row.id,
    workflowRunId: row.workflow_run_id,
    provider: row.provider,
    model: row.model,
    transportType: row.transport_type,
    providerSessionId: row.provider_session_id,
    processKey: row.process_key,
    processPid: row.process_pid,
    status: row.status,
    metadataJson: null,
    startedAt: row.started_at,
    completedAt: row.completed_at,
  };
}

export function WorkflowRunHistoryPage() {
  const location = useLocation();
  const gatewayBundle = useRef(createGatewayBundle());
  const listWorkflowRunsUseCase = useRef(
    new ListWorkflowRunsUseCase(gatewayBundle.current.workflowEngineGateway),
  );
  const listWorkflowsUseCase = useRef(
    new ListWorkflowsUseCase(gatewayBundle.current.workflowEngineGateway),
  );
  const [runs, setRuns] = useState<WorkflowRun[]>([]);
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [runTitles, setRunTitles] = useState<Map<string, string>>(new Map());
  const [projectFilter, setProjectFilter] = useState("all");
  const [scopeFilter, setScopeFilter] = useState<"all" | "global" | "private">(
    "all",
  );
  const [workflowFilter, setWorkflowFilter] = useState("all");
  const [selectedRunIds, setSelectedRunIds] = useState<string[]>([]);
  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [activeSessions, setActiveSessions] = useState<WorkflowRunSession[]>([]);
  const [isKillingAllProcesses, setIsKillingAllProcesses] = useState(false);
  const [processActionError, setProcessActionError] = useState<string | null>(null);

  async function reconcileLiveSessions(sessions: WorkflowRunSession[]) {
    if (sessions.length === 0) {
      return sessions;
    }

    let liveSessions: Array<{ processKey: string | null }> = [];
    try {
      liveSessions = await gatewayBundle.current.localRunnerGateway.listSessions();
    } catch (error) {
      console.warn("Unable to load live runner sessions for reconciliation:", error);
      return sessions;
    }

    const staleSessionIds = getMissingWorkflowRunSessionIds({
      sessions,
      liveProcessKeys: liveSessions
        .map((session) => session.processKey ?? "")
        .filter((processKey) => processKey.length > 0),
    });
    if (staleSessionIds.length === 0) {
      return sessions;
    }

    const completedAt = new Date().toISOString();
    const staleSessions = sessions.filter((session) => staleSessionIds.includes(session.id));
    try {
      await Promise.allSettled(
        staleSessions.map((session) =>
          gatewayBundle.current.localRunnerGateway.closeSession({
            transportType: session.transportType,
            providerSessionId: session.providerSessionId ?? "",
            processKey: session.processKey,
            processPid: session.processPid ?? null,
          }),
        ),
      );

      const supabase = createSupabaseBrowserClient();
      const { error } = await supabase
        .from("workflow_run_sessions")
        .update({
          status: "completed",
          completed_at: completedAt,
          process_key: null,
          process_pid: null,
        })
        .in("id", staleSessionIds);

      if (error) {
        throw new Error(error.message);
      }
    } catch (error) {
      console.warn("Unable to reconcile stale workflow sessions:", error);
      return sessions;
    }

    return markWorkflowRunSessionsCompleted(
      sessions,
      staleSessionIds,
      completedAt,
    ).filter((session) => session.status === "active" && Boolean(session.processKey));
  }

  async function loadActiveSessions() {
    const supabase = createSupabaseBrowserClient();
    const { data, error } = await supabase
      .from("workflow_run_sessions")
      .select(
        "id, workflow_run_id, provider, model, transport_type, provider_session_id, process_key, process_pid, status, started_at, completed_at",
      )
      .eq("status", "active")
      .not("process_key", "is", null);

    if (error) {
      throw new Error(`Unable to load active workflow sessions: ${error.message}`);
    }

    return await reconcileLiveSessions(
      ((data ?? []) as WorkflowRunSessionRow[]).map(mapWorkflowRunSessionRow),
    );
  }

  async function loadRunHistory() {
    const [runRows, workflowRows, projectRows] = await Promise.all([
      listWorkflowRunsUseCase.current.execute(),
      listWorkflowsUseCase.current.execute(),
      gatewayBundle.current.projectGateway.listProjects(),
    ]);
    const titles = await loadWorkflowRunTitleMap(
      gatewayBundle.current.localRunnerGateway,
      runRows.map((run) => run.id),
      {
        listArtifactRuns: () =>
          gatewayBundle.current.workflowEngineGateway.listArtifactRuns(),
        loadArtifactContent: async (artifactRun) => {
          if (!artifactRun.id.trim() || !artifactRun.remotePath.trim()) {
            return null;
          }

          const params = new URLSearchParams({
            remotePath: artifactRun.remotePath,
            storageProvider: artifactRun.storageProvider ?? "supabase",
          });
          if (artifactRun.remoteObjectId?.trim()) {
            params.set("remoteObjectId", artifactRun.remoteObjectId.trim());
          }
          if (artifactRun.projectId?.trim()) {
            params.set("projectId", artifactRun.projectId.trim());
          }

          const response = await fetch(
            `/api/local-runner/artifacts/${artifactRun.id}/open?${params.toString()}`,
          );
          if (!response.ok) {
            return null;
          }

          return await response.text();
        },
      },
    );
    setRuns(runRows);
    setWorkflows(workflowRows);
    setProjects(projectRows);
    setRunTitles(titles);
    try {
      const activeSessionRows = await loadActiveSessions();
      setActiveSessions(activeSessionRows);
    } catch (error) {
      console.error("Unable to load active workflow sessions:", error);
    }
  }

  useEffect(() => {
    if (
      location.pathname !== "/workflow-runs" &&
      location.pathname !== "/workflow-runs/"
    ) {
      return;
    }

    void loadRunHistory();
  }, [location.pathname]);

  useEffect(() => {
    const interval = setInterval(() => {
      void loadActiveSessions()
        .then((sessions) => setActiveSessions(sessions))
        .catch((error) => {
          console.error("Unable to refresh active workflow sessions:", error);
        });
    }, 5000);

    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    setSelectedRunIds([]);
  }, [projectFilter, scopeFilter, workflowFilter]);

  const workflowById = useMemo(
    () => new Map(workflows.map((workflow) => [workflow.id, workflow])),
    [workflows],
  );
  const projectNameById = useMemo(
    () => new Map(projects.map((project) => [project.id, project.name])),
    [projects],
  );

  const visibleRuns = useMemo(() => {
    return runs.filter((run) => {
      const workflow = workflowById.get(run.workflowId);
      const matchesProject =
        projectFilter === "all" || run.projectId === projectFilter;
      const matchesWorkflow =
        workflowFilter === "all" || run.workflowId === workflowFilter;
      const matchesScope =
        scopeFilter === "all" ||
        (scopeFilter === "global" && workflow?.projectId == null) ||
        (scopeFilter === "private" && workflow?.projectId != null);

      return matchesProject && matchesWorkflow && matchesScope;
    });
  }, [projectFilter, runs, scopeFilter, workflowById, workflowFilter]);

  const selectedVisibleRunIds = useMemo(
    () =>
      visibleRuns
        .filter((run) => selectedRunIds.includes(run.id))
        .map((run) => run.id),
    [selectedRunIds, visibleRuns],
  );

  const allVisibleSelected =
    visibleRuns.length > 0 && selectedVisibleRunIds.length === visibleRuns.length;

  async function deleteWorkflowRuns(runIds: string[]) {
    const uniqueRunIds = [...new Set(runIds)].filter((runId) => runId.trim().length > 0);
    if (uniqueRunIds.length === 0 || isDeleting) {
      return;
    }

    const confirmationLabel =
      uniqueRunIds.length === runs.length
        ? "all workflow runs"
        : `${uniqueRunIds.length} selected workflow run${uniqueRunIds.length === 1 ? "" : "s"}`;

    if (!window.confirm(`Delete ${confirmationLabel}? This cannot be undone.`)) {
      return;
    }

    setIsDeleting(true);
    setDeleteError(null);

    try {
      await gatewayBundle.current.workflowGateway.deleteWorkflowRuns(uniqueRunIds);
      setRuns((current) => current.filter((run) => !uniqueRunIds.includes(run.id)));
      setSelectedRunIds((current) => current.filter((runId) => !uniqueRunIds.includes(runId)));
      setActiveSessions((current) =>
        current.filter((session) => !uniqueRunIds.includes(session.workflowRunId)),
      );
    } catch (error) {
      setDeleteError(
        error instanceof Error ? error.message : "Unable to delete workflow runs.",
      );
    } finally {
      setIsDeleting(false);
    }
  }

  async function killAllProcesses() {
    const killableSessions = activeSessions.filter((session) => session.processKey);
    if (killableSessions.length === 0 || isKillingAllProcesses) {
      return;
    }

    const label = `${killableSessions.length} active process${killableSessions.length === 1 ? "" : "es"}`;
    if (!window.confirm(`Kill ${label}? This will stop every active main and replay session in the workspace.`)) {
      return;
    }

    setIsKillingAllProcesses(true);
    setProcessActionError(null);

    const completedAt = new Date().toISOString();
    const closeResults = await Promise.allSettled(
      killableSessions.map((session) =>
        gatewayBundle.current.localRunnerGateway.closeSession({
          transportType: session.transportType,
          providerSessionId: session.providerSessionId ?? "",
          processKey: session.processKey,
          processPid: session.processPid ?? null,
        }),
      ),
    );

    const closedSessionIds = killableSessions
      .filter((_, index) => closeResults[index]?.status === "fulfilled")
      .map((session) => session.id);
    const failedCount = killableSessions.length - closedSessionIds.length;

    try {
      if (closedSessionIds.length > 0) {
        const supabase = createSupabaseBrowserClient();
        const { error } = await supabase
          .from("workflow_run_sessions")
          .update({
            status: "completed",
            completed_at: completedAt,
            process_key: null,
          })
          .in("id", closedSessionIds);

        if (error) {
          throw new Error(`Unable to persist killed workflow sessions: ${error.message}`);
        }
      }

      setActiveSessions((current) =>
        current.filter((session) => !closedSessionIds.includes(session.id)),
      );

      if (failedCount > 0) {
        setProcessActionError(
          `Killed ${closedSessionIds.length} process${closedSessionIds.length === 1 ? "" : "es"}, but ${failedCount} failed.`,
        );
      }
    } catch (error) {
      setProcessActionError(
        error instanceof Error ? error.message : "Unable to kill all active processes.",
      );
      void loadActiveSessions()
        .then((sessions) => setActiveSessions(sessions))
        .catch((refreshError) => {
          console.error("Unable to reload active workflow sessions after kill failure:", refreshError);
        });
    } finally {
      setIsKillingAllProcesses(false);
    }
  }

  if (
    location.pathname !== "/workflow-runs" &&
    location.pathname !== "/workflow-runs/"
  ) {
    return <Outlet />;
  }

  return (
    <PageFrame

      title="Workflow Run History"
      description="Workspace-level history of workflow runs, with filters for workflow type, scope, and project."
      actions={
        <div className="flex flex-wrap gap-2 items-center">
          <div className="flex items-center gap-2 mr-2">
            <span className="text-sm font-medium text-success bg-background/50 border border-border/50 px-3 py-1.5 rounded-xl shadow-sm">
              <span className="text-foreground font-bold">{activeSessions.length}</span>{" "}
              active process{activeSessions.length === 1 ? "" : "es"}
            </span>
            <Button
              variant="outline"
              className="rounded-full border border-destructive/40 text-destructive hover:bg-destructive/10 hover:border-destructive/60 shadow-sm hover:shadow-md transition-all duration-300 text-[10px] font-bold uppercase tracking-wider h-7 px-3"
              disabled={activeSessions.length === 0 || isKillingAllProcesses}
              onClick={() => void killAllProcesses()}
            >
              {isKillingAllProcesses ? "Killing processes..." : "Kill All Processes"}
            </Button>
          </div>
          <Link to="/workflows" search={{ projectId: undefined }}>
            <Button variant="secondary" className="rounded-xl border border-border bg-background/50 hover:bg-muted/80 backdrop-blur-sm transition-all duration-300">
              Workflow definitions
            </Button>
          </Link>
          <Link to="/workflows/create" search={{ projectId: undefined }}>
            <Button className="rounded-xl shadow-sm hover:shadow-md hover:opacity-90 transition-all duration-300">
              Create workflow
            </Button>
          </Link>
        </div>
      }
    >
      <div className="grid gap-4 rounded-[1.5rem] border border-border/60 bg-card/45 p-5 backdrop-blur-sm md:grid-cols-3">
        <label className="space-y-2 text-sm flex flex-col">
          <span className="font-semibold text-muted-foreground mb-1">Project</span>
          <select
            className="w-full rounded-xl border border-border/80 bg-background/50 px-4 py-2.5 focus:outline-none focus:ring-1 focus:ring-primary/40 transition-all cursor-pointer font-medium"
            value={projectFilter}
            onChange={(event) => setProjectFilter(event.target.value)}
          >
            <option value="all">All projects</option>
            {projects.map((project) => (
              <option key={project.id} value={project.id}>
                {project.name}
              </option>
            ))}
          </select>
        </label>
        <label className="space-y-2 text-sm flex flex-col">
          <span className="font-semibold text-muted-foreground mb-1">Scope</span>
          <select
            className="w-full rounded-xl border border-border/80 bg-background/50 px-4 py-2.5 focus:outline-none focus:ring-1 focus:ring-primary/40 transition-all cursor-pointer font-medium"
            value={scopeFilter}
            onChange={(event) =>
              setScopeFilter(event.target.value as typeof scopeFilter)
            }
          >
            <option value="all">All scopes</option>
            <option value="global">Global workflow runs</option>
            <option value="private">Private workflow runs</option>
          </select>
        </label>
        <label className="space-y-2 text-sm flex flex-col">
          <span className="font-semibold text-muted-foreground mb-1">Workflow type</span>
          <select
            className="w-full rounded-xl border border-border/80 bg-background/50 px-4 py-2.5 focus:outline-none focus:ring-1 focus:ring-primary/40 transition-all cursor-pointer font-medium"
            value={workflowFilter}
            onChange={(event) => setWorkflowFilter(event.target.value)}
          >
            <option value="all">All workflows</option>
            {workflows.map((workflow) => (
              <option key={workflow.id} value={workflow.id}>
                {workflow.name}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div className="flex flex-wrap items-center gap-4 rounded-[1.5rem] border border-border/60 bg-card/45 p-4 backdrop-blur-sm shadow-sm">
        <label className="flex items-center gap-3 text-sm font-medium text-muted-foreground cursor-pointer select-none">
          <input
            checked={allVisibleSelected}
            className="h-4 w-4 rounded border-border/80 bg-background/50 text-primary focus:ring-primary/40 transition-all cursor-pointer"
            disabled={visibleRuns.length === 0 || isDeleting}
            onChange={() =>
              setSelectedRunIds(
                allVisibleSelected ? [] : visibleRuns.map((run) => run.id),
              )
            }
            type="checkbox"
          />
          <span>Select all visible</span>
        </label>

        <div className="ml-auto flex flex-wrap gap-2">
          <Button
            className="border border-red-500/20 text-red-600 hover:bg-red-500/10 hover:border-red-500/40 rounded-xl px-4 py-2 text-sm font-semibold transition-all duration-300"
            disabled={selectedVisibleRunIds.length === 0 || isDeleting}
            onClick={() => void deleteWorkflowRuns(selectedVisibleRunIds)}
            variant="secondary"
          >
            {isDeleting
              ? "Deleting..."
              : `Delete selected${selectedVisibleRunIds.length > 0 ? ` (${selectedVisibleRunIds.length})` : ""}`}
          </Button>
          <Button
            className="border border-red-500/20 text-red-600 hover:bg-red-500/10 hover:border-red-500/40 rounded-xl px-4 py-2 text-sm font-semibold transition-all duration-300"
            disabled={runs.length === 0 || isDeleting}
            onClick={() => void deleteWorkflowRuns(runs.map((run) => run.id))}
            variant="secondary"
          >
            Delete all
          </Button>
        </div>
      </div>

      {deleteError ? (
        <div className="rounded-[1.5rem] border border-red-500/20 bg-red-500/10 p-4 text-sm text-red-700 font-medium">
          {deleteError}
        </div>
      ) : null}

      {processActionError ? (
        <div className="rounded-[1.5rem] border border-red-500/20 bg-red-500/10 p-4 text-sm text-red-700 font-medium">
          {processActionError}
        </div>
      ) : null}

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        {visibleRuns.length === 0 ? (
          <div className="rounded-[1.5rem] border border-dashed border-border bg-background/60 p-6 text-sm text-muted-foreground md:col-span-2 lg:col-span-3">
            No workflow runs match the current filters.
          </div>
        ) : null}
        {visibleRuns.map((run) => {
          const workflow = workflowById.get(run.workflowId);
          const runTitle = runTitles.get(run.id);
          const selected = selectedRunIds.includes(run.id);
          const status = (run.status || "").toLowerCase();

          let gradientClass = "from-violet-500/80 via-purple-500/80 to-blue-500/80";
          let statusBadgeClass = "bg-muted/50 text-muted-foreground border-border/50";
          if (status === "completed" || status === "success") {
            gradientClass = "from-emerald-500/80 via-teal-500/80 to-cyan-500/80";
            statusBadgeClass = "bg-emerald-500/10 text-emerald-500 border-emerald-500/20";
          } else if (status === "running" || status === "active" || status === "pending") {
            gradientClass = "from-blue-500/80 via-sky-500/80 to-cyan-500/80";
            statusBadgeClass = "bg-blue-500/10 text-blue-500 border-blue-500/20";
          } else if (status === "failed" || status === "error") {
            gradientClass = "from-red-500/80 via-rose-500/80 to-orange-500/80";
            statusBadgeClass = "bg-red-500/10 text-red-500 border-red-500/20";
          }

          return (
            <div
              key={run.id}
              className={`group relative flex flex-col justify-between overflow-hidden rounded-[1.8rem] border bg-gradient-to-b from-card/90 to-background/40 backdrop-blur-md p-6 shadow-sm hover:shadow-xl hover:border-primary/20 hover:-translate-y-1 transition-all duration-300 ${
                selected ? "border-primary/40 shadow-md ring-1 ring-primary/25" : "border-border/60"
              }`}
            >
              {/* Dynamic top gradient line based on status */}
              <div className={`absolute top-0 left-0 right-0 h-[3px] opacity-70 group-hover:opacity-100 transition-opacity bg-gradient-to-r ${gradientClass}`} />

              <div className="flex flex-col h-full">
                {/* Header with selection checkbox */}
                <div className="flex items-start justify-between gap-3 mb-4">
                  <div className="flex items-start gap-2.5 min-w-0">
                    <label className="mt-1 flex shrink-0 items-center cursor-pointer">
                      <input
                        checked={selected}
                        className="h-4 w-4 rounded border-border/80 bg-background/50 text-primary focus:ring-primary/40 transition-all cursor-pointer"
                        disabled={isDeleting}
                        onChange={() =>
                          setSelectedRunIds((current) =>
                            current.includes(run.id)
                              ? current.filter((id) => id !== run.id)
                              : [...current, run.id],
                          )
                        }
                        type="checkbox"
                      />
                    </label>
                    <div className="min-w-0">
                      <Link
                        to="/workflow-runs/$runId"
                        params={{ runId: run.id }}
                        className="block hover:underline"
                      >
                        <h3 className="text-lg font-bold tracking-tight text-foreground group-hover:text-primary transition-colors duration-300 truncate">
                          {runTitle ?? workflow?.name ?? run.workflowId}
                        </h3>
                      </Link>
                      <p className="font-mono text-[9px] tracking-wider text-muted-foreground uppercase mt-1 truncate">
                        ID: {run.id}
                      </p>
                    </div>
                  </div>
                  <div className="flex flex-col items-end gap-1.5 shrink-0">
                    <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold border uppercase tracking-wider ${statusBadgeClass}`}>
                      {run.status}
                    </span>
                    <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold border border-border/50 bg-muted/30 text-muted-foreground uppercase tracking-wider`}>
                      {workflow?.projectId ? "Private" : "Global"}
                    </span>
                  </div>
                </div>

                {/* Details Area */}
                <div className="space-y-2.5 border-t border-border/40 pt-4 mb-6 text-xs text-muted-foreground/90">
                  <p className="font-medium text-foreground/75 truncate">
                    Project: <span className="font-normal text-muted-foreground">{projectNameById.get(run.projectId) ?? run.projectId}</span>
                  </p>
                  <p className="font-medium text-foreground/75 truncate">
                    Started: <span className="font-normal text-muted-foreground">{new Date(run.startedAt).toLocaleString()}</span>
                  </p>
                </div>
              </div>

              {/* Actions Footer */}
              <div className="flex items-center justify-between pt-2 border-t border-border/40 mt-auto">
                <Button
                  className="border border-red-500/20 text-red-600 hover:bg-red-500/10 hover:border-red-500/40 rounded-xl px-2.5 py-1 h-8 text-[11px] font-semibold transition-all duration-300"
                  disabled={isDeleting}
                  onClick={() => void deleteWorkflowRuns([run.id])}
                  variant="secondary"
                >
                  Delete
                </Button>
                <Link
                  to="/workflow-runs/$runId"
                  params={{ runId: run.id }}
                >
                  <Button size="sm" variant="secondary" className="rounded-xl bg-muted/60 hover:bg-primary hover:text-primary-foreground border border-border/40 h-8 text-xs transition-all duration-300 group/btn">
                    Open run
                    <ArrowRight className="ml-1.5 h-3.5 w-3.5 group-hover/btn:translate-x-1 transition-transform duration-300" />
                  </Button>
                </Link>
              </div>
            </div>
          );
        })}
      </div>
    </PageFrame>
  );
}
