import { useEffect, useMemo, useRef, useState } from "react";
import { createFileRoute, Link, Outlet, useLocation } from "@tanstack/react-router";
import { ArrowRight } from "lucide-react";

import { PageFrame } from "@/components/common/page-frame";
import { Button } from "@/components/ui/button";
import { createGatewayBundle } from "@/data/repository/browser-factory";
import { ListWorkflowsUseCase } from "@/domain/usecase/workflow-engine/list-workflows-usecase";
import type { Workflow } from "@/domain/model/entity/workflow-engine";
import type { Project } from "@/domain/model/entity/project";

type WorkflowSortOption = "name-asc" | "name-desc" | "updated-asc" | "updated-desc";

export const Route = createFileRoute("/_authenticated/workflows")({
  validateSearch: (search: Record<string, unknown>) => ({
    projectId: typeof search.projectId === "string" ? search.projectId : undefined,
  }),
  component: WorkflowDefinitionsPage,
});

export function WorkflowDefinitionsPage() {
  const location = useLocation();
  const { projectId: searchProjectId } = Route.useSearch();
  const gatewayBundle = useRef(createGatewayBundle());
  const listWorkflowsUseCase = useRef(
    new ListWorkflowsUseCase(gatewayBundle.current.workflowEngineGateway)
  );
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [scopeFilter, setScopeFilter] = useState<"all" | "global" | "private">("all");
  const [projectFilter, setProjectFilter] = useState(searchProjectId ?? "all");
  const [sortBy, setSortBy] = useState<WorkflowSortOption>("updated-desc");

  useEffect(() => {
    setProjectFilter(searchProjectId ?? "all");
  }, [searchProjectId]);

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      try {
        const [workflowRows, projectRows] = await Promise.all([
          listWorkflowsUseCase.current.execute(),
          gatewayBundle.current.projectGateway.listProjects(),
        ]);
        setWorkflows(workflowRows);
        setProjects(projectRows);
      } finally {
        setLoading(false);
      }
    };

    void load();
  }, []);

  const projectNameById = useMemo(
    () => new Map(projects.map((project) => [project.id, project.name])),
    [projects]
  );

  const visibleWorkflows = useMemo(() => {
    const filtered = workflows.filter((workflow) => {
      const matchesScope =
        scopeFilter === "all" ||
        (scopeFilter === "global" && workflow.projectId === null) ||
        (scopeFilter === "private" && workflow.projectId !== null);
      const matchesProject =
        projectFilter === "all" || workflow.projectId === projectFilter;

      return matchesScope && matchesProject;
    });

    return filtered.slice().sort((left, right) => {
      switch (sortBy) {
        case "name-asc":
          return left.name.localeCompare(right.name);
        case "name-desc":
          return right.name.localeCompare(left.name);
        case "updated-asc":
          return new Date(left.updatedAt).getTime() - new Date(right.updatedAt).getTime();
        case "updated-desc":
        default:
          return new Date(right.updatedAt).getTime() - new Date(left.updatedAt).getTime();
      }
    });
  }, [projectFilter, scopeFilter, sortBy, workflows]);

  if (location.pathname !== "/workflows") {
    return <Outlet />;
  }

  return (
    <PageFrame
      title="Workflow Definitions"
      description="Browse all workflow definitions, including built-in global templates and private project-owned workflows."
      actions={
        <div className="flex flex-wrap gap-2">
          <Link to="/workflow-steps">
            <Button variant="secondary">Step catalog</Button>
          </Link>
          <Link to="/workflow-runs">
            <Button variant="secondary">Run history</Button>
          </Link>
          <Link to="/workflows/create" search={{ projectId: searchProjectId }}>
            <Button>Create workflow</Button>
          </Link>
        </div>
      }
    >
      <div className="grid gap-4 rounded-[1.5rem] border border-border bg-background/60 p-5 md:grid-cols-3">
        <label className="space-y-2 text-sm">
          <span className="font-medium">Scope</span>
          <select
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            value={scopeFilter}
            onChange={(event) => setScopeFilter(event.target.value as typeof scopeFilter)}
          >
            <option value="all">All definitions</option>
            <option value="global">Global only</option>
            <option value="private">Private only</option>
          </select>
        </label>
        <label className="space-y-2 text-sm">
          <span className="font-medium">Owner project</span>
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
          <span className="font-medium">Sort by</span>
          <select
            className="w-full rounded-2xl border border-border bg-card px-4 py-3"
            value={sortBy}
            onChange={(event) => setSortBy(event.target.value as WorkflowSortOption)}
          >
            <option value="name-asc">Alphabet (A-Z)</option>
            <option value="name-desc">Alphabet (Z-A)</option>
            <option value="updated-desc">Updated time (Newest first)</option>
            <option value="updated-asc">Updated time (Oldest first)</option>
          </select>
        </label>
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        {loading ? (
          <div className="rounded-[1.5rem] border border-dashed border-border bg-background/60 p-5 text-sm text-muted-foreground xl:col-span-2">
            Loading workflow definitions...
          </div>
        ) : null}
        {!loading && visibleWorkflows.length === 0 ? (
          <div className="rounded-[1.5rem] border border-dashed border-border bg-background/60 p-5 text-sm text-muted-foreground xl:col-span-2">
            No workflow definitions match the current filters.
          </div>
        ) : null}
        {visibleWorkflows.map((workflow) => (
          <div
            key={workflow.id}
            className="rounded-[1.5rem] border border-border bg-background/60 p-5"
          >
            <div className="flex items-start justify-between gap-3">
              <div>
                <h3 className="text-xl font-semibold">{workflow.name}</h3>
                <p className="mt-2 text-sm text-muted-foreground">{workflow.description}</p>
              </div>
              <div className="flex flex-wrap gap-2">
                {workflow.isTemplate ? (
                  <span className="rounded-full bg-accent px-3 py-1 text-xs font-semibold text-accent-foreground">
                    Built-in
                  </span>
                ) : null}
                <span className="rounded-full border border-border px-3 py-1 text-xs font-semibold text-muted-foreground">
                  {workflow.projectId ? "Private" : "Global"}
                </span>
              </div>
            </div>
            <p className="mt-4 text-xs text-muted-foreground">
              Owner: {workflow.projectId ? projectNameById.get(workflow.projectId) ?? workflow.projectId : "Workspace global"}
            </p>
            <div className="mt-4 flex flex-wrap gap-2">
              <Link
                params={{ workflowId: workflow.id }}
                search={{ projectId: projectFilter !== "all" ? projectFilter : undefined }}
                to="/workflows/$workflowId"
              >
                <Button variant="secondary">
                  Open workflow
                  <ArrowRight className="ml-2 h-4 w-4" />
                </Button>
              </Link>
            </div>
          </div>
        ))}
      </div>
    </PageFrame>
  );
}
