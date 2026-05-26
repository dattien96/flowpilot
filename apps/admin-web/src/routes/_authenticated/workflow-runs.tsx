import { useEffect, useMemo, useRef, useState } from "react";
import {
  createFileRoute,
  Link,
  Outlet,
  useLocation,
} from "@tanstack/react-router";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { ListWorkflowRunsUseCase } from "@/domain/usecase/workflow-engine/list-workflow-runs-usecase";
import { ListWorkflowsUseCase } from "@/domain/usecase/workflow-engine/list-workflows-usecase";
import type { Project } from "@/domain/model/entity/project";
import type {
  Workflow,
  WorkflowRun,
} from "@/domain/model/entity/workflow-engine";
import { loadWorkflowRunTitleMap } from "@/lib/workflow-run-title";

export const Route = createFileRoute("/_authenticated/workflow-runs")({
  component: WorkflowRunHistoryPage,
});

function WorkflowRunHistoryPage() {
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

  useEffect(() => {
    const load = async () => {
      const [runRows, workflowRows, projectRows] = await Promise.all([
        listWorkflowRunsUseCase.current.execute(),
        listWorkflowsUseCase.current.execute(),
        gatewayBundle.current.projectGateway.listProjects(),
      ]);
      const titles = await loadWorkflowRunTitleMap(
        gatewayBundle.current.localRunnerGateway,
        runRows.map((run) => run.id),
      );
      setRuns(runRows);
      setWorkflows(workflowRows);
      setProjects(projectRows);
      setRunTitles(titles);
    };

    void load();
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
      await gatewayBundle.current.workflowEngineGateway.deleteWorkflowRuns(uniqueRunIds);
      setRuns((current) => current.filter((run) => !uniqueRunIds.includes(run.id)));
      setSelectedRunIds((current) => current.filter((runId) => !uniqueRunIds.includes(runId)));
    } catch (error) {
      setDeleteError(
        error instanceof Error ? error.message : "Unable to delete workflow runs.",
      );
    } finally {
      setIsDeleting(false);
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
        <div className="flex flex-wrap gap-2">
          <Link to="/workflows" search={{ projectId: undefined }}>
            <Button variant="secondary">Workflow definitions</Button>
          </Link>
          <Link to="/workflows/create" search={{ projectId: undefined }}>
            <Button>Create workflow</Button>
          </Link>
        </div>
      }
    >
      <div className="grid gap-4 rounded-[1.5rem] border border-border bg-background/60 p-5 md:grid-cols-3">
        <label className="space-y-2 text-sm">
          <span className="font-medium">Project</span>
          <select
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
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
        <label className="space-y-2 text-sm">
          <span className="font-medium">Scope</span>
          <select
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
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
        <label className="space-y-2 text-sm">
          <span className="font-medium">Workflow type</span>
          <select
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
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

      <div className="flex flex-wrap items-center gap-3 rounded-[1.5rem] border border-border bg-background/60 p-4">
        <label className="flex items-center gap-3 text-sm text-muted-foreground">
          <input
            checked={allVisibleSelected}
            className="h-4 w-4"
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
            className="border border-red-500/30 text-red-600 hover:bg-red-500/10"
            disabled={selectedVisibleRunIds.length === 0 || isDeleting}
            onClick={() => void deleteWorkflowRuns(selectedVisibleRunIds)}
            variant="secondary"
          >
            {isDeleting
              ? "Deleting..."
              : `Delete selected${selectedVisibleRunIds.length > 0 ? ` (${selectedVisibleRunIds.length})` : ""}`}
          </Button>
          <Button
            className="border border-red-500/30 text-red-600 hover:bg-red-500/10"
            disabled={runs.length === 0 || isDeleting}
            onClick={() => void deleteWorkflowRuns(runs.map((run) => run.id))}
            variant="secondary"
          >
            Delete all
          </Button>
        </div>
      </div>

      {deleteError ? (
        <div className="rounded-[1.5rem] border border-red-500/20 bg-red-500/10 p-4 text-sm text-red-700">
          {deleteError}
        </div>
      ) : null}

      <div className="space-y-4">
        {visibleRuns.length === 0 ? (
          <div className="rounded-[1.5rem] border border-dashed border-border bg-background/60 p-5 text-sm text-muted-foreground">
            No workflow runs match the current filters.
          </div>
        ) : null}
        {visibleRuns.map((run) => {
          const workflow = workflowById.get(run.workflowId);
          const runTitle = runTitles.get(run.id);
          const selected = selectedRunIds.includes(run.id);

          return (
            <div
              key={run.id}
            >
              <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
                <div className="flex flex-wrap items-start gap-3">
                  <label className="mt-1 flex shrink-0 items-center">
                    <input
                      checked={selected}
                      className="h-4 w-4"
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
                  <Link
                    to="/workflow-runs/$runId"
                    params={{ runId: run.id }}
                    className="min-w-0 flex-1 transition-all hover:opacity-90 hover:-translate-y-0.5"
                  >
                    <h3 className="text-xl font-semibold">
                      {runTitle ?? workflow?.name ?? run.workflowId}
                    </h3>
                    {runTitle ? (
                      <p className="mt-2 break-all font-mono text-xs text-muted-foreground">
                        Run ID: {run.id}
                      </p>
                    ) : null}
                    <p className="mt-2 text-sm text-muted-foreground">
                      Project:{" "}
                      {projectNameById.get(run.projectId) ?? run.projectId}
                    </p>
                    <p className="mt-1 text-sm text-muted-foreground">
                      Started: {new Date(run.startedAt).toLocaleString()}
                    </p>
                  </Link>
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="rounded-full border border-border px-3 py-1 text-xs font-semibold text-muted-foreground">
                      {workflow?.projectId ? "Private" : "Global"}
                    </span>
                    <span className="rounded-full bg-accent px-3 py-1 text-xs font-semibold text-accent-foreground">
                      {run.status}
                    </span>
                    <Button
                      className="border border-red-500/30 text-red-600 hover:bg-red-500/10"
                      disabled={isDeleting}
                      onClick={() => void deleteWorkflowRuns([run.id])}
                      variant="secondary"
                    >
                      Delete
                    </Button>
                  </div>
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </PageFrame>
  );
}
