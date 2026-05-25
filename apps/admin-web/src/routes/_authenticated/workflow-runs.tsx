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

      <div className="space-y-4">
        {visibleRuns.length === 0 ? (
          <div className="rounded-[1.5rem] border border-dashed border-border bg-background/60 p-5 text-sm text-muted-foreground">
            No workflow runs match the current filters.
          </div>
        ) : null}
        {visibleRuns.map((run) => {
          const workflow = workflowById.get(run.workflowId);
          const runTitle = runTitles.get(run.id);

          return (
            <Link
              key={run.id}
              to="/workflow-runs/$runId"
              params={{ runId: run.id }}
              className="block transition-all hover:opacity-90 hover:-translate-y-0.5"
            >
              <div className="rounded-[1.5rem] border border-border bg-background/60 p-5">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div>
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
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <span className="rounded-full border border-border px-3 py-1 text-xs font-semibold text-muted-foreground">
                      {workflow?.projectId ? "Private" : "Global"}
                    </span>
                    <span className="rounded-full bg-accent px-3 py-1 text-xs font-semibold text-accent-foreground">
                      {run.status}
                    </span>
                  </div>
                </div>
              </div>
            </Link>
          );
        })}
      </div>
    </PageFrame>
  );
}
